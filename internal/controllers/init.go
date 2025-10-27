// Package controllers 提供传输层 Handler，负责处理外部请求并调用业务层。
// 该层负责参数校验、DTO 转换和错误映射。
package controllers

import "github.com/google/wire"

// ProviderSet exposes controller/handler constructors for DI.
var ProviderSet = wire.NewSet(
	NewBaseHandler,
	NewLifecycleHandler,
	NewVideoQueryHandler,
)
