package service

import (
	"context"
	"fmt"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/biz"

	"github.com/go-kratos/kratos/v2/metadata"
	"github.com/go-kratos/kratos/v2/transport"
)

// GreeterService is a greeter service.
type GreeterService struct {
	v1.UnimplementedGreeterServer

	uc *biz.GreeterUsecase
}

const forwardedHeader = "x-template-forwarded"

// NewGreeterService new a greeter service.
func NewGreeterService(uc *biz.GreeterUsecase) *GreeterService {
	return &GreeterService{uc: uc}
}

// SayHello implements helloworld.GreeterServer.
func (s *GreeterService) SayHello(ctx context.Context, in *v1.HelloRequest) (*v1.HelloReply, error) {
	g, err := s.uc.CreateGreeter(ctx, &biz.Greeter{Hello: in.Name})
	if err != nil {
		return nil, err
	}
	message := "Hello " + g.Hello

	if !isForwarded(ctx) {
		forwardCtx := ensureClientMetadata(ctx)
		forwardCtx = metadata.AppendToClientContext(forwardCtx, forwardedHeader, "true")
		if remoteMsg, err := s.uc.ForwardHello(forwardCtx, in.Name); err == nil && remoteMsg != "" {
			message = fmt.Sprintf("%s | remote: %s", message, remoteMsg)
		}
	}

	return &v1.HelloReply{Message: message}, nil
}

func isForwarded(ctx context.Context) bool {
	if tr, ok := transport.FromServerContext(ctx); ok {
		if hdr := tr.RequestHeader(); hdr != nil && hdr.Get(forwardedHeader) != "" {
			return true
		}
	}
	if md, ok := metadata.FromServerContext(ctx); ok {
		return md.Get(forwardedHeader) != ""
	}
	return false
}

func ensureClientMetadata(ctx context.Context) context.Context {
	if _, ok := metadata.FromClientContext(ctx); ok {
		return ctx
	}
	return metadata.NewClientContext(ctx, metadata.Metadata{})
}
