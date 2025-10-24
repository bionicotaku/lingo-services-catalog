package database

import "github.com/google/wire"

// ProviderSet 暴露数据库连接池构造器供 Wire 依赖注入使用。
var ProviderSet = wire.NewSet(
	NewPgxPool,
)
