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

// Event 描述 Profile Outbox 发布的用户互动事件。
type Event struct {
	EventName      string    `json:"event_name"`
	State          string    `json:"state"`
	EngagementType string    `json:"engagement_type"`
	UserID         string    `json:"user_id"`
	VideoID        string    `json:"video_id"`
	Source         string    `json:"source"`
	OccurredAt     time.Time `json:"occurred_at"`
	Version        string    `json:"version"`
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
		EventName:      pb.GetEventName(),
		UserID:         strings.TrimSpace(pb.GetUserId()),
		VideoID:        strings.TrimSpace(pb.GetVideoId()),
		EngagementType: pb.GetEngagementType(),
		State:          pb.GetState(),
		Source:         pb.GetSource(),
		OccurredAt: func() time.Time {
			if ts := pb.GetOccurredAt(); ts != nil {
				return ts.AsTime().UTC()
			}
			return time.Time{}
		}(),
		Version: pb.GetVersion(),
	}
	normalizeEvent(evt)
	return evt, nil
}

// normalizeEvent 补足缺省值并确保 OccurredAt/Version 合法。
func normalizeEvent(evt *Event) {
	evt.EventName = strings.TrimSpace(evt.EventName)
	evt.State = strings.ToLower(strings.TrimSpace(evt.State))
	evt.EngagementType = strings.ToLower(strings.TrimSpace(evt.EngagementType))
	evt.UserID = strings.TrimSpace(evt.UserID)
	evt.VideoID = strings.TrimSpace(evt.VideoID)
	evt.Source = strings.TrimSpace(evt.Source)

	if evt.EventName == "" && evt.State != "" {
		evt.EventName = "profile.engagement." + evt.State
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now().UTC()
	} else {
		evt.OccurredAt = evt.OccurredAt.UTC()
	}
	if strings.TrimSpace(evt.Version) == "" {
		evt.Version = EventVersion
	}
}

// actionType 表示订阅事件的操作类型。
type actionType string

const (
	actionUnknown actionType = ""
	actionAdded   actionType = "added"
	actionRemoved actionType = "removed"
)

// resolveAction 解析事件动作（新增/移除）。
func (e *Event) resolveAction() (actionType, error) {
	if action := parseAction(e.State); action != actionUnknown {
		return action, nil
	}
	if action := parseAction(deriveSuffix(e.EventName)); action != actionUnknown {
		return action, nil
	}
	return actionUnknown, fmt.Errorf("engagement: unsupported action from event_name=%q state=%q", e.EventName, e.State)
}

func parseAction(v string) actionType {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "added", "add":
		return actionAdded
	case "removed", "remove", "deleted", "delete":
		return actionRemoved
	default:
		return actionUnknown
	}
}

func deriveSuffix(eventName string) string {
	eventName = strings.ToLower(strings.TrimSpace(eventName))
	if eventName == "" {
		return ""
	}
	parts := strings.Split(eventName, ".")
	return parts[len(parts)-1]
}

// engagementKind 表示互动类型。
type engagementKind string

const (
	kindUnknown  engagementKind = ""
	kindLike     engagementKind = "like"
	kindBookmark engagementKind = "bookmark"
)

// resolveKind 返回事件的互动类型。
func (e *Event) resolveKind() (engagementKind, error) {
	switch strings.ToLower(strings.TrimSpace(e.EngagementType)) {
	case "like":
		return kindLike, nil
	case "bookmark", "favorite", "favourite":
		return kindBookmark, nil
	default:
		return kindUnknown, fmt.Errorf("engagement: unsupported engagement_type=%q", e.EngagementType)
	}
}
