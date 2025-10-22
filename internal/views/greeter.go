// Package views 提供视图对象（VO）与 API DTO（Proto 消息）之间的转换辅助函数。
// 负责将 Service 层返回的 VO 渲染为 Proto 响应，保持 Controller 层的精简。
package views

import (
	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
)

// NewHelloReply 将 Greeting 视图对象转换为 gRPC API 响应消息。
// 处理 nil 情况，返回空的 HelloReply 以避免 panic。
func NewHelloReply(greeting *vo.Greeting) *v1.HelloReply {
	if greeting == nil {
		return &v1.HelloReply{}
	}
	return &v1.HelloReply{Message: greeting.Message}
}
