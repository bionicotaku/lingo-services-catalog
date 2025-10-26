package outboxevents

import (
	"errors"
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
)

// ErrEmptyUpdatePayload 表示没有任何字段需要更新。
var ErrEmptyUpdatePayload = errors.New("event builder: empty update payload")

// VideoUpdateChanges 描述更新事件中需携带的字段。
type VideoUpdateChanges struct {
	Title             *string
	Description       *string
	Status            *po.VideoStatus
	MediaStatus       *po.StageStatus
	AnalysisStatus    *po.StageStatus
	DurationMicros    *int64
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	RawSubtitleURL    *string
}

// NewVideoUpdatedEvent 基于更新后的实体与变更集构建领域事件。
func NewVideoUpdatedEvent(video *po.Video, changes VideoUpdateChanges, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}

	if occurredAt.IsZero() {
		occurredAt = video.UpdatedAt
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}
	}

	occurredAt = occurredAt.UTC()
	version := VersionFromTime(occurredAt)

	payload := &VideoUpdated{
		VideoID: video.VideoID,
		Tags:    nil,
	}
	hasChange := false

	if changes.Title != nil {
		payload.Title = changes.Title
		hasChange = true
	}
	if changes.Description != nil {
		payload.Description = changes.Description
		hasChange = true
	}
	if changes.Status != nil {
		value := string(*changes.Status)
		payload.Status = &value
		hasChange = true
	}
	if changes.MediaStatus != nil {
		value := string(*changes.MediaStatus)
		payload.MediaStatus = &value
		hasChange = true
	}
	if changes.AnalysisStatus != nil {
		value := string(*changes.AnalysisStatus)
		payload.AnalysisStatus = &value
		hasChange = true
	}
	if changes.DurationMicros != nil {
		payload.DurationMicros = changes.DurationMicros
		hasChange = true
	}
	if changes.ThumbnailURL != nil {
		payload.ThumbnailURL = changes.ThumbnailURL
		hasChange = true
	}
	if changes.HLSMasterPlaylist != nil {
		payload.HLSMasterPlaylist = changes.HLSMasterPlaylist
		hasChange = true
	}
	if changes.Difficulty != nil {
		payload.Difficulty = changes.Difficulty
		hasChange = true
	}
	if changes.Summary != nil {
		payload.Summary = changes.Summary
		hasChange = true
	}
	if changes.RawSubtitleURL != nil {
		payload.RawSubtitleURL = changes.RawSubtitleURL
		hasChange = true
	}

	if !hasChange {
		return nil, ErrEmptyUpdatePayload
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoUpdated,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       version,
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}
