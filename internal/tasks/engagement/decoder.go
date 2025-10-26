// Package engagement contains ingestion utilities for engagement projections.
package engagement

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
)

// EventVersion 表示 Engagement 事件协议的版本常量。
const EventVersion = "v1"

// Event 描述 Engagement 服务发布的用户互动事件。
type Event struct {
	UserID        string    `json:"user_id"`
	VideoID       string    `json:"video_id"`
	HasLiked      *bool     `json:"has_liked,omitempty"`
	HasBookmarked *bool     `json:"has_bookmarked,omitempty"`
	HasWatched    *bool     `json:"has_watched,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	Version       string    `json:"version"`
}

// eventDecoder 支持 Proto 与 JSON 的双模解码。
type eventDecoder struct{}

func newEventDecoder() *eventDecoder {
	return &eventDecoder{}
}

// Decode 将原始消息解码为 Event。优先尝试 Proto，回退 JSON。
func (d *eventDecoder) Decode(data []byte) (*Event, error) {
	if evt, err := decodeProto(data); err == nil {
		return evt, nil
	}

	var evtJSON Event
	if err := json.Unmarshal(data, &evtJSON); err != nil {
		return nil, fmt.Errorf("engagement: decode payload: %w", err)
	}
	normalizeEvent(&evtJSON)
	return &evtJSON, nil
}

// decodeProto 解析 protobuf 载荷。
func decodeProto(data []byte) (*Event, error) {
	var pb EventProto
	if err := proto.Unmarshal(data, &pb); err != nil {
		return nil, err
	}
	evt := &Event{
		UserID:  strings.TrimSpace(pb.GetUserId()),
		VideoID: strings.TrimSpace(pb.GetVideoId()),
		OccurredAt: func() time.Time {
			if ts := pb.GetOccurredAt(); ts != nil {
				return ts.AsTime().UTC()
			}
			return time.Time{}
		}(),
		Version: pb.GetVersion(),
	}
	if pb.HasLiked != nil {
		value := pb.GetHasLiked()
		evt.HasLiked = &value
	}
	if pb.HasBookmarked != nil {
		value := pb.GetHasBookmarked()
		evt.HasBookmarked = &value
	}
	if pb.HasWatched != nil {
		value := pb.GetHasWatched()
		evt.HasWatched = &value
	}
	normalizeEvent(evt)
	return evt, nil
}

// normalizeEvent 补足缺省值并确保 OccurredAt/Version 合法。
func normalizeEvent(evt *Event) {
	evt.UserID = strings.TrimSpace(evt.UserID)
	evt.VideoID = strings.TrimSpace(evt.VideoID)
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now().UTC()
	} else {
		evt.OccurredAt = evt.OccurredAt.UTC()
	}
	if strings.TrimSpace(evt.Version) == "" {
		evt.Version = EventVersion
	}
}
