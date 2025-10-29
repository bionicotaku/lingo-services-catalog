// Package engagement contains ingestion utilities for engagement projections.
package engagement

import "fmt"

// DecodePayload wraps the internal decoder to expose parsing logic for tests and helpers.
func DecodePayload(data []byte) (*Event, error) {
	return newEventDecoder().Decode(data)
}

// Event 包装原始事件字节，供 Handler 按 event_type 解析。
type Event struct {
	Payload []byte
}

type eventDecoder struct{}

func newEventDecoder() *eventDecoder {
	return &eventDecoder{}
}

// Decode 将原始消息封装为 Event，保持 payload 供 Handler 解码。
func (d *eventDecoder) Decode(data []byte) (*Event, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("engagement: empty payload")
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	return &Event{Payload: buf}, nil
}
