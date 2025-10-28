package controllers

import (
	"context"
	"strings"
	"time"

	metadata "github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// HandlerType 表示 Handler 的语义类别，用于选择超时策略。
type HandlerType int

const (
	// HandlerTypeDefault 表示未显式区分的 Handler。
	HandlerTypeDefault HandlerType = iota
	// HandlerTypeCommand 表示写模型命令 Handler。
	HandlerTypeCommand
	// HandlerTypeQuery 表示读模型查询 Handler。
	HandlerTypeQuery
)

// HandlerTimeouts 聚合不同类型 Handler 的超时策略。
type HandlerTimeouts struct {
	Default time.Duration
	Command time.Duration
	Query   time.Duration
}

const (
	fallbackDefaultTimeout = 5 * time.Second
	fallbackQueryTimeout   = 3 * time.Second
	headerUserInfo         = "x-apigateway-api-userinfo"
	headerIdempotencyKey   = "x-md-idempotency-key"
	headerIfMatch          = "x-md-if-match"
	headerIfNoneMatch      = "x-md-if-none-match"
)

// BaseHandler 提供公共的超时、Metadata 解析能力，供具体 Handler 内嵌复用。
type BaseHandler struct {
	timeouts HandlerTimeouts
}

// NewBaseHandler 构造基础 Handler，并为缺省值填充合理的回退策略。
func NewBaseHandler(timeouts HandlerTimeouts) *BaseHandler {
	if timeouts.Default <= 0 {
		if timeouts.Command > 0 {
			timeouts.Default = timeouts.Command
		} else if timeouts.Query > 0 {
			timeouts.Default = timeouts.Query
		} else {
			timeouts.Default = fallbackDefaultTimeout
		}
	}
	if timeouts.Command <= 0 {
		timeouts.Command = timeouts.Default
	}
	if timeouts.Query <= 0 {
		if timeouts.Default > 0 {
			timeouts.Query = timeouts.Default
		} else {
			timeouts.Query = fallbackQueryTimeout
		}
	}
	return &BaseHandler{timeouts: timeouts}
}

// WithTimeout 根据 Handler 类型包装上下文，返回绑定超时的新 Context 与取消函数。
func (h *BaseHandler) WithTimeout(ctx context.Context, kind HandlerType) (context.Context, context.CancelFunc) {
	if h == nil {
		return context.WithTimeout(ctx, fallbackDefaultTimeout)
	}
	var timeout time.Duration
	switch kind {
	case HandlerTypeCommand:
		timeout = h.timeouts.Command
	case HandlerTypeQuery:
		timeout = h.timeouts.Query
	default:
		timeout = h.timeouts.Default
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// ExtractMetadata 解析请求中常见的幂等与条件请求 Header。
func (h *BaseHandler) ExtractMetadata(ctx context.Context) metadata.HandlerMetadata {
	md, ok := grpcmetadata.FromIncomingContext(ctx)
	if !ok {
		return metadata.HandlerMetadata{}
	}
	meta := metadata.HandlerMetadata{
		IdempotencyKey: firstMetadata(md, headerIdempotencyKey),
		IfMatch:        firstMetadata(md, headerIfMatch),
		IfNoneMatch:    firstMetadata(md, headerIfNoneMatch),
	}
	rawUserInfo := firstMetadata(md, headerUserInfo)
	meta.RawUserInfo = rawUserInfo
	if rawUserInfo != "" {
		if userID, err := metadata.ExtractUserIDFromUserInfo(rawUserInfo); err == nil {
			if strings.TrimSpace(userID) != "" {
				meta.UserID = userID
			} else {
				meta.InvalidUserInfo = true
			}
		} else {
			meta.InvalidUserInfo = true
		}
	}
	return meta
}

// InjectHandlerMetadata 将解析结果注入到 Context，供后续层访问。
func InjectHandlerMetadata(ctx context.Context, meta metadata.HandlerMetadata) context.Context {
	return metadata.Inject(ctx, meta)
}

// HandlerMetadataFromContext 读取上游注入的 HandlerMetadata。
func HandlerMetadataFromContext(ctx context.Context) (metadata.HandlerMetadata, bool) {
	return metadata.FromContext(ctx)
}

func firstMetadata(md grpcmetadata.MD, key string) string {
	if len(md) == 0 {
		return ""
	}
	values := md.Get(strings.ToLower(key))
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
