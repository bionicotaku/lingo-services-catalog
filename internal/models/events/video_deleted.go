package events

import (
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewVideoDeletedEvent 基于删除的实体构建 VideoDeleted 事件。
func NewVideoDeletedEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time, reason *string) (*videov1.Event, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}

	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	version := VersionFromTime(occurredAt)
	ts := timestamppb.New(occurredAt.UTC())
	payload := &videov1.VideoDeleted{
		VideoId:    video.VideoID.String(),
		Version:    version,
		DeletedAt:  ts,
		OccurredAt: ts,
	}
	if reason != nil {
		payload.Reason = wrapperspb.String(*reason)
	}

	event := &videov1.Event{
		EventId:       eventID.String(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_DELETED,
		AggregateId:   video.VideoID.String(),
		AggregateType: AggregateTypeVideo,
		Version:       version,
		OccurredAt:    ts,
		Payload:       &videov1.Event_Deleted{Deleted: payload},
	}
	return event, nil
}
