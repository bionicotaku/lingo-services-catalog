// Package controllers 提供传输层 Handler，负责处理外部请求并调用业务层。
package controllers

import (
	"context"
	"fmt"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/internal/views"

	"github.com/go-kratos/kratos/v2/metadata"
)

// GreeterHandler 是 Greeter 服务的 gRPC 传输层处理器。
// 负责将 Proto 请求转换为业务层调用，并将结果渲染为 Proto 响应。
type GreeterHandler struct {
	v1.UnimplementedGreeterServer

	uc *services.GreeterUsecase // 注入的业务用例层
}

// forwardedHeader 是用于标记请求已被转发的元数据键，防止递归调用。
const forwardedHeader = "x-template-forwarded"

// NewGreeterHandler 构造一个由 GreeterUsecase 支撑的 gRPC Handler。
func NewGreeterHandler(uc *services.GreeterUsecase) *GreeterHandler {
	return &GreeterHandler{uc: uc}
}

// SayHello 实现 helloworld.GreeterServer 接口，处理 SayHello RPC 调用。
//
// 业务流程：
// 1. 调用业务层创建本地问候语
// 2. 如果请求未被标记为转发，则尝试调用远程服务并合并结果
// 3. 将结果通过 views 层转换为 Proto 响应
//
// 防止递归调用：通过检查 metadata 中的 forwardedHeader，避免服务间无限循环调用。
func (s *GreeterHandler) SayHello(ctx context.Context, in *v1.HelloRequest) (*v1.HelloReply, error) {
	// 1. 创建本地问候语
	greeting, err := s.uc.CreateGreeting(ctx, in.GetName())
	if err != nil {
		return nil, err
	}

	message := greeting.Message

	// 2. 尝试转发到远程服务（仅当请求未被标记为已转发时）
	if !isForwarded(ctx) {
		forwardCtx := ensureClientMetadata(ctx)
		forwardCtx = metadata.AppendToClientContext(forwardCtx, forwardedHeader, "true")
		if remoteMsg, err := s.uc.ForwardHello(forwardCtx, in.GetName()); err == nil && remoteMsg != "" {
			message = fmt.Sprintf("%s | remote: %s", message, remoteMsg)
		}
	}

	// 3. 渲染响应
	greeting.Message = message
	return views.NewHelloReply(greeting), nil
}

// isForwarded 检查请求是否已被标记为转发，用于防止递归调用。
func isForwarded(ctx context.Context) bool {
	if md, ok := metadata.FromServerContext(ctx); ok {
		return md.Get(forwardedHeader) != ""
	}
	return false
}

// ensureClientMetadata 确保上下文中存在客户端元数据，用于向下游服务传递 metadata。
func ensureClientMetadata(ctx context.Context) context.Context {
	if _, ok := metadata.FromClientContext(ctx); ok {
		return ctx
	}
	return metadata.NewClientContext(ctx, metadata.Metadata{})
}
