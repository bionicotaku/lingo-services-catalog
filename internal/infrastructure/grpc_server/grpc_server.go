// Package grpcserver 负责装配入站 gRPC Server 及其中间件栈。
// 包括：追踪、日志、限流、校验、恢复等中间件，以及可选的指标采集。
package grpcserver

import (
	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"

	"github.com/bionicotaku/lingo-utils/observability"
	obsTrace "github.com/bionicotaku/lingo-utils/observability/tracing"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/ratelimit"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/validate"
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
// 5. validate.Validator() - Proto-Gen-Validate 参数校验
// 6. logging.Server() - 结构化日志记录（含 trace_id/span_id）
//
// 可选指标采集：
// - 根据 metricsCfg.GRPCEnabled 决定是否启用 otelgrpc.StatsHandler
// - 可通过 metricsCfg.GRPCIncludeHealth 控制是否采集健康检查指标
func NewGRPCServer(c *configpb.Server, metricsCfg *observability.MetricsConfig, greeter *controllers.GreeterHandler, logger log.Logger) *grpc.Server {
	// metricsCfg 为可选参数，默认启用指标采集以保持向后兼容。
	// 调用方可通过配置显式控制指标行为。
	metricsEnabled := true
	includeHealth := false
	if metricsCfg != nil {
		metricsEnabled = metricsCfg.GRPCEnabled
		includeHealth = metricsCfg.GRPCIncludeHealth
	}

	opts := []grpc.ServerOption{
		grpc.Middleware(
			obsTrace.Server(),
			recovery.Recovery(),
			metadata.Server(
				metadata.WithPropagatedPrefix("x-template-"),
			),
			ratelimit.Server(),
			validate.Validator(),
			logging.Server(logger),
		),
	}
	if metricsEnabled {
		handler := newServerHandler(includeHealth)
		opts = append(opts, grpc.Options(stdgrpc.StatsHandler(handler)))
	}
	if c.GetGrpc().GetNetwork() != "" {
		opts = append(opts, grpc.Network(c.GetGrpc().GetNetwork()))
	}
	if c.GetGrpc().GetAddr() != "" {
		opts = append(opts, grpc.Address(c.GetGrpc().GetAddr()))
	}
	if c.GetGrpc().GetTimeout() != nil {
		opts = append(opts, grpc.Timeout(c.GetGrpc().GetTimeout().AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	v1.RegisterGreeterServer(srv, greeter)
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
