package outboxevents

import (
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/google/uuid"
)

// NewVideoCreatedEvent 基于持久化实体构建领域事件。
func NewVideoCreatedEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}
	if occurredAt.IsZero() {
		occurredAt = video.CreatedAt
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}
	}

	occurredAt = occurredAt.UTC()
	version := VersionFromTime(occurredAt)

	payload := &VideoCreated{
		VideoID:        video.VideoID,
		UploaderID:     video.UploadUserID,
		Title:          video.Title,
		Description:    video.Description,
		DurationMicros: video.DurationMicros,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoCreated,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       version,
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}
