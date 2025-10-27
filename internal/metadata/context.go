// Package metadata 提供 HandlerMetadata 在 Context 中的存取工具，供控制器与服务层共享。
package metadata

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// HandlerMetadata 描述从请求头或上游链路解析出的上下文信息。
type HandlerMetadata struct {
	IdempotencyKey string
	IfMatch        string
	IfNoneMatch    string
	UserID         string
}

// IsZero 判断 Metadata 是否为空。
func (m HandlerMetadata) IsZero() bool {
	return m.IdempotencyKey == "" &&
		m.IfMatch == "" &&
		m.IfNoneMatch == "" &&
		m.UserID == ""
}

// UserUUID 尝试解析 user_id 为 UUID。
func (m HandlerMetadata) UserUUID() (uuid.UUID, bool) {
	if strings.TrimSpace(m.UserID) == "" {
		return uuid.Nil, false
	}
	value, err := uuid.Parse(m.UserID)
	if err != nil {
		return uuid.Nil, false
	}
	return value, true
}

type ctxKey struct{}

// Inject 将 HandlerMetadata 注入 Context。
func Inject(ctx context.Context, meta HandlerMetadata) context.Context {
	if meta.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, meta)
}

// FromContext 读取上游注入的 HandlerMetadata。
func FromContext(ctx context.Context) (HandlerMetadata, bool) {
	if ctx == nil {
		return HandlerMetadata{}, false
	}
	meta, ok := ctx.Value(ctxKey{}).(HandlerMetadata)
	return meta, ok
}
