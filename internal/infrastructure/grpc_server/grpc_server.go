// Package grpcserver 负责装配入站 gRPC Server 及其中间件栈。
// 包括：追踪、日志、限流、校验、恢复等中间件，以及可选的指标采集。
package grpcserver

import (
	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	obsTrace "github.com/bionicotaku/lingo-utils/observability/tracing"
	pvmw "github.com/go-kratos-ecosystem/components/v2/middleware/protovalidate"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/ratelimit"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	otelgrpcfilters "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"go.opentelemetry.io/otel"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// NewGRPCServer 构造配置完整的 Kratos gRPC Server 实例。
//
// 中间件链（按执行顺序）：
// 1. obsTrace.Server() - OpenTelemetry 追踪，自动创建 Span
// 2. recovery.Recovery() - Panic 恢复，防止服务崩溃
// 3. metadata.Server() - 元数据传播，转发 x-template- 前缀的 header
// 4. ratelimit.Server() - 限流保护
// 5. pvmw.Server() - protovalidate 运行时参数校验（基于反射，无需代码生成）
// 6. logging.Server() - 结构化日志记录（含 trace_id/span_id）
//
// 可选指标采集：
// - 根据 metricsCfg.GRPCEnabled 决定是否启用 otelgrpc.StatsHandler
// - 可通过 metricsCfg.GRPCIncludeHealth 控制是否采集健康检查指标
func NewGRPCServer(cfg configloader.ServerConfig, metricsCfg *observability.MetricsConfig, jwt gcjwt.ServerMiddleware, lifecycle *controllers.LifecycleHandler, query *controllers.VideoQueryHandler, logger log.Logger) *grpc.Server {
	// metricsCfg 为可选参数，默认启用指标采集以保持向后兼容。
	// 调用方可通过配置显式控制指标行为。
	metricsEnabled := true
	includeHealth := false
	if metricsCfg != nil {
		metricsEnabled = metricsCfg.GRPCEnabled
		includeHealth = metricsCfg.GRPCIncludeHealth
	}

	// 构造基础中间件链：追踪、panic 恢复与 metadata 传播。
	mws := []middleware.Middleware{
		obsTrace.Server(),
		recovery.Recovery(),
		metadata.Server(metadata.WithPropagatedPrefix(cfg.MetadataKeys...)),
	}
	// 根据配置决定是否挂载 JWT 校验，默认置于限流之前。
	if jwt != nil {
		mws = append(mws, middleware.Middleware(jwt))
	}
	// 其余中间件保持原有顺序，保护限流、参数校验与结构化日志逻辑。
	mws = append(mws,
		ratelimit.Server(),
		pvmw.Server(), // protovalidate 运行时验证（无需代码生成）
		logging.Server(logger),
	)

	opts := []grpc.ServerOption{
		grpc.Middleware(mws...),
	}
	if metricsEnabled {
		handler := newServerHandler(includeHealth)
		opts = append(opts, grpc.Options(stdgrpc.StatsHandler(handler)))
	}
	if cfg.Network != "" {
		opts = append(opts, grpc.Network(cfg.Network))
	}
	if cfg.Address != "" {
		opts = append(opts, grpc.Address(cfg.Address))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, grpc.Timeout(cfg.Timeout))
	}
	srv := grpc.NewServer(opts...)
	if query != nil {
		videov1.RegisterCatalogQueryServiceServer(srv, query)
	}
	if lifecycle != nil {
		videov1.RegisterCatalogLifecycleServiceServer(srv, lifecycle)
	}
	return srv
}

// newServerHandler 构造 gRPC Server 的 OpenTelemetry StatsHandler。
//
// 参数：
//   - includeHealth: 是否采集健康检查 RPC 的指标
//     false 时会过滤 /grpc.health.v1.Health/Check，减少指标噪音
//
// 返回配置好的 StatsHandler，用于采集 RPC 指标（延迟、错误率等）。
func newServerHandler(includeHealth bool) stats.Handler {
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(otel.GetMeterProvider()),
	}
	if !includeHealth {
		opts = append(opts, otelgrpc.WithFilter(otelgrpcfilters.Not(otelgrpcfilters.HealthCheck())))
	}
	return otelgrpc.NewServerHandler(opts...)
}
