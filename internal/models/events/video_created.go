package events

import (
	"errors"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	// AggregateTypeVideo 标识视频聚合类型，供 Outbox headers / attributes 使用。
	AggregateTypeVideo = "video"
	// SchemaVersionV1 描述事件载荷的当前 schema 版本。
	SchemaVersionV1 = "v1"
)

var (
	// ErrNilVideo 在构建事件时视频实体为空。
	ErrNilVideo = errors.New("event builder: video is nil")
	// ErrInvalidEventID 表示未提供合法的事件 ID。
	ErrInvalidEventID = errors.New("event builder: event id is required")
)

// NewVideoCreatedEvent 基于持久化实体构建 VideoCreated 事件。
func NewVideoCreatedEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time) (*videov1.Event, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	ts := timestamppb.New(occurredAt.UTC())
	payload := &videov1.VideoCreated{
		VideoId:        video.VideoID.String(),
		UploaderId:     video.UploadUserID.String(),
		Title:          video.Title,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
		Version:        1,
		OccurredAt:     ts,
	}

	if video.Description != nil {
		payload.Description = wrapperspb.String(*video.Description)
	}
	if video.DurationMicros != nil {
		payload.DurationMicros = wrapperspb.Int64(*video.DurationMicros)
	}

	event := &videov1.Event{
		EventId:       eventID.String(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_CREATED,
		AggregateId:   video.VideoID.String(),
		AggregateType: AggregateTypeVideo,
		Version:       1,
		OccurredAt:    ts,
		Payload:       &videov1.Event_Created{Created: payload},
	}
	return event, nil
}
