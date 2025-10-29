package repositories

import "github.com/google/wire"

// ProviderSet 暴露 Repository 层的构造函数供 Wire 依赖注入使用。
// 包含所有 Repository 的构造器。
var ProviderSet = wire.NewSet(
	NewVideoRepository,  // ← Video 仓储（基于 sqlc）
	NewOutboxRepository, // ← Outbox 仓储
	NewInboxRepository,  // ← Inbox 仓储
	NewVideoUserStatesRepository,
)
