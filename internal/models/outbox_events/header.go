// Package outboxevents 提供领域事件相关的元数据辅助方法。本文件专注于
// 从领域事件中派生 Pub/Sub/Outbox 所需的附加属性（attributes）及共用工具。
package outboxevents

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// FormatEventType 将事件种类映射为语义化字符串。
func FormatEventType(kind Kind) string {
	return kind.String()
}

// BuildAttributes 构造符合 Pub/Sub 约定的 message attributes。
func BuildAttributes(event *DomainEvent, schemaVersion string, traceID string) map[string]string {
	if schemaVersion == "" {
		schemaVersion = SchemaVersionV1
	}
	attrs := map[string]string{
		"event_id":       event.EventID.String(),
		"event_type":     FormatEventType(event.Kind),
		"aggregate_id":   event.AggregateID.String(),
		"aggregate_type": event.AggregateType,
		"version":        strconv.FormatInt(event.Version, 10),
		"occurred_at":    event.OccurredAt.UTC().Format(time.RFC3339Nano),
		"schema_version": schemaVersion,
	}
	if traceID != "" {
		attrs["trace_id"] = traceID
	}
	return attrs
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
