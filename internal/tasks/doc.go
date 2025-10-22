// Package tasks 承载后台工作任务，如 Outbox 扫描器、定时调度器、异步 Worker 等。
// 需要初始化逻辑时，在本目录创建 init.go 并通过 Wire ProviderSet 注册。
package tasks
