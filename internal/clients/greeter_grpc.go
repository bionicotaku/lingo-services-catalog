// Package clients 包含调用外部服务的客户端门面（Façade），封装 gRPC/REST 调用细节。
// 实现 Service 层定义的 Remote 接口，提供业务级别的调用抽象。
package clients

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc"
)

// greeterRemote 是 services.GreeterRemote 接口的实现，封装远程 Greeter 服务调用。
type greeterRemote struct {
	client v1.GreeterClient // gRPC 客户端桩（由 grpc.ClientConn 生成）
	log    *log.Helper      // 结构化日志辅助器
}

// NewGreeterRemote 构造 GreeterRemote 接口实现，封装共享的 gRPC 连接。
// 支持优雅降级：如果 conn 为 nil（未配置远程服务），返回无操作实现。
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

// SayHello 调用远程 Greeter 服务的 SayHello RPC。
// 如果客户端未初始化，记录警告日志并返回空字符串（不报错），允许服务优雅降级。
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
