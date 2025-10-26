// Package grpcclient 负责配置出站 gRPC 连接，供服务调用下游依赖使用。
// 包括：追踪、熔断、元数据传播等中间件，以及可选的指标采集。
package grpcclient

import (
	"context"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	obsTrace "github.com/bionicotaku/lingo-utils/observability/tracing"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/circuitbreaker"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	otelgrpcfilters "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// NewGRPCClient 创建配置完整的 gRPC 客户端连接。
//
// 中间件链（按执行顺序）：
// 1. recovery.Recovery() - 捕获客户端调用中的 panic
// 2. metadata.Client() - 自动传播 metadata 到下游
// 3. obsTrace.Client() - OpenTelemetry 追踪，创建子 Span
// 4. circuitbreaker.Client() - 熔断保护，防止雪崩
//
// 可选指标采集：
// - 根据 metricsCfg.GRPCEnabled 决定是否启用 otelgrpc.StatsHandler
//
// 返回值：
// - *grpc.ClientConn: 可复用的连接实例
// - cleanup func(): 清理函数，应在服务关闭时调用
// - error: 连接失败时返回错误
//
// 特殊处理：
// - 如果未配置 target，返回 nil conn（不报错），允许服务在无下游依赖时启动
func NewGRPCClient(cfg configloader.GRPCClientConfig, metricsCfg *observability.MetricsConfig, jwt gcjwt.ClientMiddleware, logger log.Logger) (*grpc.ClientConn, func(), error) {
	helper := log.NewHelper(logger)

	// 如果未配置目标地址，返回 nil 连接（不报错）
	// 这允许服务在开发环境或无远程依赖时正常启动
	if cfg.Target == "" {
		helper.Warn("grpc client target not configured; remote calls disabled")
		return nil, func() {}, nil
	}

	// metricsCfg 为可选参数，默认启用指标采集以保持向后兼容
	metricsEnabled := true
	includeHealth := false
	if metricsCfg != nil {
		metricsEnabled = metricsCfg.GRPCEnabled
		includeHealth = metricsCfg.GRPCIncludeHealth
	}

	// 基础中间件链：panic 恢复 + metadata 传播，确保下游能收到必要头信息。
	mws := []middleware.Middleware{
		recovery.Recovery(),
		metadata.Client(metadata.WithPropagatedPrefix(cfg.MetadataKeys...)),
	}
	// 按需注入 JWT，中间件只在配置启用时生效。
	if jwt != nil && !cfg.JWT.Disabled {
		mws = append(mws, middleware.Middleware(jwt))
	}
	// 追踪与熔断保留原顺序，保证链路观测与保护能力。
	mws = append(mws,
		obsTrace.Client(),
		circuitbreaker.Client(),
	)

	opts := []kgrpc.ClientOption{
		kgrpc.WithEndpoint(cfg.Target),
		kgrpc.WithMiddleware(mws...),
	}
	if metricsEnabled {
		opts = append(opts, kgrpc.WithOptions(grpc.WithStatsHandler(newClientHandler(includeHealth))))
	}

	conn, err := kgrpc.DialInsecure(context.Background(), opts...)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if err := conn.Close(); err != nil {
			helper.Errorf("close grpc client: %v", err)
		}
	}

	return conn, cleanup, nil
}

// newClientHandler 构造 gRPC Client 的 OpenTelemetry StatsHandler。
//
// 参数：
//   - includeHealth: 是否采集健康检查 RPC 的指标
//     false 时会过滤 /grpc.health.v1.Health/Check，减少指标噪音
//
// 返回配置好的 StatsHandler，用于采集客户端 RPC 指标（延迟、错误率等）。
func newClientHandler(includeHealth bool) stats.Handler {
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(otel.GetMeterProvider()),
	}
	if !includeHealth {
		opts = append(opts, otelgrpc.WithFilter(otelgrpcfilters.Not(otelgrpcfilters.HealthCheck())))
	}
	return otelgrpc.NewClientHandler(opts...)
}
