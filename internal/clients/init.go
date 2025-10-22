package clients

import "github.com/google/wire"

// ProviderSet 暴露 Clients 层的构造函数供 Wire 依赖注入使用。
// 当前包含：GreeterRemote 的构造器。
var ProviderSet = wire.NewSet(NewGreeterRemote)
