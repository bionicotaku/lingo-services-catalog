package clients

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc"
)

type greeterRemote struct {
	client v1.GreeterClient
	log    *log.Helper
}

// NewGreeterRemote wraps the shared gRPC client connection with a Greeter-specific facade.
func NewGreeterRemote(conn *grpc.ClientConn, logger log.Logger) services.GreeterRemote {
	helper := log.NewHelper(logger)
	if conn == nil {
		helper.Warn("no grpc client connection; greeter remote disabled")
		return &greeterRemote{log: helper}
	}
	return &greeterRemote{
		client: v1.NewGreeterClient(conn),
		log:    helper,
	}
}

func (r *greeterRemote) SayHello(ctx context.Context, name string) (string, error) {
	if r.client == nil {
		r.log.WithContext(ctx).Warn("greeter remote client not initialized")
		return "", nil
	}
	reply, err := r.client.SayHello(ctx, &v1.HelloRequest{Name: name})
	if err != nil {
		return "", err
	}
	return reply.GetMessage(), nil
}
