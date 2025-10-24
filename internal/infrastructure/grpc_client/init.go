package grpcclient

import "github.com/google/wire"

// ProviderSet 暴露 gRPC Client 连接的构造函数供 Wire 依赖注入使用。
var ProviderSet = wire.NewSet(NewGRPCClient)
