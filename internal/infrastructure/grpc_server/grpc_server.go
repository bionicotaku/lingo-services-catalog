// Package grpcserver wires the inbound gRPC server and its middleware stack.
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

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *configpb.Server, metricsCfg *observability.MetricsConfig, greeter *controllers.GreeterHandler, logger log.Logger) *grpc.Server {
	// metricsCfg is optional; default to enabled metrics so callers that omit
	// the configuration still get a functional server with tracing only.
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

func newServerHandler(includeHealth bool) stats.Handler {
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(otel.GetMeterProvider()),
	}
	if !includeHealth {
		opts = append(opts, otelgrpc.WithFilter(otelgrpcfilters.Not(otelgrpcfilters.HealthCheck())))
	}
	return otelgrpc.NewServerHandler(opts...)
}
