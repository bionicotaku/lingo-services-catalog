// Package events 提供领域事件构造与元数据辅助函数，统一事件命名与属性。
package events

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"go.opentelemetry.io/otel/trace"
)

// FormatEventType 将枚举映射为语义化字符串（如 video.created）。
func FormatEventType(eventType videov1.EventType) string {
	switch eventType {
	case videov1.EventType_EVENT_TYPE_VIDEO_CREATED:
		return "video.created"
	case videov1.EventType_EVENT_TYPE_VIDEO_UPDATED:
		return "video.updated"
	case videov1.EventType_EVENT_TYPE_VIDEO_DELETED:
		return "video.deleted"
	default:
		return "video.unknown"
	}
}

// BuildAttributes 构造符合 Pub/Sub 约定的 message attributes。
func BuildAttributes(event *videov1.Event, schemaVersion string, traceID string) map[string]string {
	if schemaVersion == "" {
		schemaVersion = SchemaVersionV1
	}
	attrs := map[string]string{
		"event_id":       event.GetEventId(),
		"event_type":     FormatEventType(event.GetEventType()),
		"aggregate_id":   event.GetAggregateId(),
		"aggregate_type": event.GetAggregateType(),
		"version":        strconv.FormatInt(event.GetVersion(), 10),
		"occurred_at":    event.GetOccurredAt().AsTime().UTC().Format(time.RFC3339),
		"schema_version": schemaVersion,
	}
	if traceID != "" {
		attrs["trace_id"] = traceID
	}
	return attrs
}

// MarshalAttributes 将 attributes 编码为 JSON，供 outbox.headers 字段使用。
func MarshalAttributes(attrs map[string]string) ([]byte, error) {
	return json.Marshal(attrs)
}

// TraceIDFromContext 提取 OTel Trace ID，若不存在返回空字符串。
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() || !spanCtx.HasTraceID() {
		return ""
	}
	return spanCtx.TraceID().String()
}

// VersionFromTime 根据时间戳计算聚合版本号，采用 UTC 微秒时间，保证单调递增。
func VersionFromTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMicro()
}
