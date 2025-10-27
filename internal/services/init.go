// Package services 包含应用业务用例的编排逻辑。
// 该层负责协调 Repository 和 Clients，实现核心业务规则，不直接依赖传输层或基础设施细节。
package services

import "github.com/google/wire"

// ProviderSet 暴露 Services 层的构造函数供 Wire 依赖注入使用。
// 包含所有 Usecase 的构造器。
var ProviderSet = wire.NewSet(
	NewLifecycleWriter,
	NewVideoQueryService,
	NewRegisterUploadService,
	NewOriginalMediaService,
	NewProcessingStatusService,
	NewMediaInfoService,
	NewAIAttributesService,
	NewVisibilityService,
	NewLifecycleService,
)
