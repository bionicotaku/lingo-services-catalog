// Package grpcclient configures outbound gRPC connections used by services.
package grpcclient

import (
	"context"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"

	"github.com/bionicotaku/lingo-utils/observability"
	obsTrace "github.com/bionicotaku/lingo-utils/observability/tracing"
	"github.com/go-kratos/kratos/v2/log"
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

// NewGRPCClient dials a generic gRPC target with common Kratos middlewares applied.
func NewGRPCClient(c *configpb.Data, metricsCfg *observability.MetricsConfig, logger log.Logger) (*grpc.ClientConn, func(), error) {
	helper := log.NewHelper(logger)

	if c == nil || c.GrpcClient == nil || c.GrpcClient.Target == "" {
		helper.Warn("grpc client target not configured; remote calls disabled")
		return nil, func() {}, nil
	}

	// metricsCfg may be nil when callers do not configure metrics explicitly;
	// default to enabling instrumentation so behaviour matches template defaults.
	metricsEnabled := true
	includeHealth := false
	if metricsCfg != nil {
		metricsEnabled = metricsCfg.GRPCEnabled
		includeHealth = metricsCfg.GRPCIncludeHealth
	}

	opts := []kgrpc.ClientOption{
		kgrpc.WithEndpoint(c.GrpcClient.Target),
		kgrpc.WithMiddleware(
			recovery.Recovery(),
			metadata.Client(),
			obsTrace.Client(),
			circuitbreaker.Client(),
		),
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

func newClientHandler(includeHealth bool) stats.Handler {
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(otel.GetMeterProvider()),
	}
	if !includeHealth {
		opts = append(opts, otelgrpc.WithFilter(otelgrpcfilters.Not(otelgrpcfilters.HealthCheck())))
	}
	return otelgrpc.NewClientHandler(opts...)
}
