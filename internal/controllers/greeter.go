package controllers

import (
	"context"
	"fmt"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/internal/views"

	"github.com/go-kratos/kratos/v2/metadata"
)

// GreeterController handles Greeter transport logic.
type GreeterController struct {
	v1.UnimplementedGreeterServer

	uc *services.GreeterUsecase
}

const forwardedHeader = "x-template-forwarded"

// NewGreeterController constructs a controller backed by GreeterUsecase.
func NewGreeterController(uc *services.GreeterUsecase) *GreeterController {
	return &GreeterController{uc: uc}
}

// SayHello implements helloworld.GreeterServer.
func (s *GreeterController) SayHello(ctx context.Context, in *v1.HelloRequest) (*v1.HelloReply, error) {
	greeting, err := s.uc.CreateGreeting(ctx, in.GetName())
	if err != nil {
		return nil, err
	}

	message := greeting.Message
	if !isForwarded(ctx) {
		forwardCtx := ensureClientMetadata(ctx)
		forwardCtx = metadata.AppendToClientContext(forwardCtx, forwardedHeader, "true")
		if remoteMsg, err := s.uc.ForwardHello(forwardCtx, in.GetName()); err == nil && remoteMsg != "" {
			message = fmt.Sprintf("%s | remote: %s", message, remoteMsg)
		}
	}

	greeting.Message = message
	return views.NewHelloReply(greeting), nil
}

func isForwarded(ctx context.Context) bool {
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
