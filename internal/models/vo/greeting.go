// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

// Greeting 封装返回给 API 消费者的问候消息。
// 由 Service 层构造，避免直接暴露 PO（数据库实体）。
type Greeting struct {
	Message string // 格式化后的问候语
}
