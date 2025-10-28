// Package metadata 提供 HandlerMetadata 在 Context 中的存取工具，供控制器与服务层共享。
package metadata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// HandlerMetadata 描述从请求头或上游链路解析出的上下文信息。
type HandlerMetadata struct {
	IdempotencyKey  string
	IfMatch         string
	IfNoneMatch     string
	UserID          string
	RawUserInfo     string
	InvalidUserInfo bool
}

// IsZero 判断 Metadata 是否为空。
func (m HandlerMetadata) IsZero() bool {
	return m.IdempotencyKey == "" &&
		m.IfMatch == "" &&
		m.IfNoneMatch == "" &&
		m.UserID == "" &&
		m.RawUserInfo == "" &&
		!m.InvalidUserInfo
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

// ExtractUserIDFromUserInfo 尝试从 X-Apigateway-Api-Userinfo 头中解析用户标识。
func ExtractUserIDFromUserInfo(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	payload, err := decodeUserInfo(raw)
	if err != nil {
		return "", err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}
	if sub, ok := claims["sub"].(string); ok && strings.TrimSpace(sub) != "" {
		return sub, nil
	}
	if userID, ok := claims["user_id"].(string); ok && strings.TrimSpace(userID) != "" {
		return userID, nil
	}
	if uid, ok := claims["uid"].(string); ok && strings.TrimSpace(uid) != "" {
		return uid, nil
	}
	return "", nil
}

func decodeUserInfo(raw string) ([]byte, error) {
	decoders := []func(string) ([]byte, error){
		func(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) },
		func(s string) ([]byte, error) { return base64.URLEncoding.DecodeString(s) },
		func(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) },
	}
	var err error
	for _, decode := range decoders {
		var payload []byte
		payload, err = decode(raw)
		if err == nil {
			return payload, nil
		}
	}
	return nil, errors.New("decode userinfo header failed")
}
