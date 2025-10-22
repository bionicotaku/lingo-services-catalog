package repositories

import "github.com/google/wire"

// ProviderSet 暴露 Repository 层的构造函数供 Wire 依赖注入使用。
// 当前包含：GreeterRepo 的构造器。
// 后续接入数据库时，可在此添加 NewDB 等基础设施 Provider。
var ProviderSet = wire.NewSet(
	NewGreeterRepo,
)
