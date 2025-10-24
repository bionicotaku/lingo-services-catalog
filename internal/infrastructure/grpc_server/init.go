package grpcserver

import "github.com/google/wire"

// ProviderSet 暴露 gRPC Server 的构造函数供 Wire 依赖注入使用。
var ProviderSet = wire.NewSet(NewGRPCServer)
