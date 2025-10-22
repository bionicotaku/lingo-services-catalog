// Package views provides conversion helpers between view objects and API DTOs.
package views

import (
	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
)

// NewHelloReply converts a Greeting view object into the API response message.
func NewHelloReply(greeting *vo.Greeting) *v1.HelloReply {
	if greeting == nil {
		return &v1.HelloReply{}
	}
	return &v1.HelloReply{Message: greeting.Message}
}
