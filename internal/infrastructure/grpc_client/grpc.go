// Package grpcclient configures outbound gRPC connections used by services.
package grpcclient

import (
	"context"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/circuitbreaker"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
)

// NewGRPCClient dials a generic gRPC target with common Kratos middlewares applied.
func NewGRPCClient(c *configpb.Data, logger log.Logger) (*grpc.ClientConn, func(), error) {
	helper := log.NewHelper(logger)

	if c == nil || c.GrpcClient == nil || c.GrpcClient.Target == "" {
		helper.Warn("grpc client target not configured; remote calls disabled")
		return nil, func() {}, nil
	}

	conn, err := kgrpc.DialInsecure(context.Background(),
		kgrpc.WithEndpoint(c.GrpcClient.Target),
		kgrpc.WithMiddleware(
			recovery.Recovery(),
			metadata.Client(),
			tracing.Client(),
			circuitbreaker.Client(),
		),
	)
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
