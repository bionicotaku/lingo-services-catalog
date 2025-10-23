// Package clients 封装与外部服务的交互客户端。
// 该层负责将外部 gRPC/REST 调用封装为业务层接口。
package clients

import "github.com/google/wire"

// ProviderSet 暴露 Clients 层的构造函数供 Wire 依赖注入使用。
// 当需要添加外部服务客户端时，在此注册构造器。
var ProviderSet = wire.NewSet()
