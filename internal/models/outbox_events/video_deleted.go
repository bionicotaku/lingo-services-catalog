package outboxevents

import (
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/google/uuid"
)

// NewVideoDeletedEvent 基于删除的实体构建领域事件。
func NewVideoDeletedEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time, reason *string) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}

	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	occurredAt = occurredAt.UTC()
	version := VersionFromTime(occurredAt)

	deletedAt := occurredAt
	payload := &VideoDeleted{
		VideoID:   video.VideoID,
		DeletedAt: &deletedAt,
		Reason:    reason,
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoDeleted,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       version,
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}
