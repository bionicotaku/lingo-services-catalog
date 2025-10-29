package outboxevents

import (
	"errors"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
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
	VisibilityStatus  *string
	PublishAt         *time.Time
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
	if changes.VisibilityStatus != nil {
		value := *changes.VisibilityStatus
		payload.VisibilityStatus = &value
		hasChange = true
	}
	if changes.PublishAt != nil {
		payload.PublishedAt = cloneTime(changes.PublishAt)
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

// NewVideoMediaReadyEvent 基于更新后的实体构建媒体阶段完成事件。
func NewVideoMediaReadyEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}
	if video.MediaStatus != po.StageReady {
		return nil, fmt.Errorf("event builder: media stage not ready")
	}

	if occurredAt.IsZero() {
		switch {
		case video.MediaEmittedAt != nil:
			occurredAt = video.MediaEmittedAt.UTC()
		case !video.UpdatedAt.IsZero():
			occurredAt = video.UpdatedAt.UTC()
		default:
			occurredAt = time.Now().UTC()
		}
	} else {
		occurredAt = occurredAt.UTC()
	}

	payload := &VideoMediaReady{
		VideoID:           video.VideoID,
		Status:            video.Status,
		MediaStatus:       video.MediaStatus,
		AnalysisStatus:    video.AnalysisStatus,
		DurationMicros:    video.DurationMicros,
		EncodedResolution: video.EncodedResolution,
		EncodedBitrate:    video.EncodedBitrate,
		ThumbnailURL:      video.ThumbnailURL,
		HLSMasterPlaylist: video.HLSMasterPlaylist,
		JobID:             video.MediaJobID,
		EmittedAt:         cloneTime(video.MediaEmittedAt),
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoMediaReady,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       VersionFromTime(occurredAt),
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}

// NewVideoAIEnrichedEvent 基于更新后的实体构建 AI 阶段完成事件。
func NewVideoAIEnrichedEvent(video *po.Video, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}
	if video.AnalysisStatus != po.StageReady {
		return nil, fmt.Errorf("event builder: analysis stage not ready")
	}

	if occurredAt.IsZero() {
		switch {
		case video.AnalysisEmittedAt != nil:
			occurredAt = video.AnalysisEmittedAt.UTC()
		case !video.UpdatedAt.IsZero():
			occurredAt = video.UpdatedAt.UTC()
		default:
			occurredAt = time.Now().UTC()
		}
	} else {
		occurredAt = occurredAt.UTC()
	}

	tags := append([]string(nil), video.Tags...)

	payload := &VideoAIEnriched{
		VideoID:        video.VideoID,
		Status:         video.Status,
		AnalysisStatus: video.AnalysisStatus,
		MediaStatus:    video.MediaStatus,
		Difficulty:     video.Difficulty,
		Summary:        video.Summary,
		Tags:           tags,
		RawSubtitleURL: video.RawSubtitleURL,
		JobID:          video.AnalysisJobID,
		EmittedAt:      cloneTime(video.AnalysisEmittedAt),
		ErrorMessage:   video.ErrorMessage,
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoAIEnriched,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       VersionFromTime(occurredAt),
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}

// NewVideoProcessingFailedEvent 构建处理失败事件。
func NewVideoProcessingFailedEvent(video *po.Video, stage string, jobID *string, emittedAt *time.Time, errorMessage *string, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}
	switch stage {
	case "media", "analysis":
	default:
		return nil, ErrInvalidStage
	}

	if occurredAt.IsZero() {
		switch {
		case emittedAt != nil:
			occurredAt = emittedAt.UTC()
		case !video.UpdatedAt.IsZero():
			occurredAt = video.UpdatedAt.UTC()
		default:
			occurredAt = time.Now().UTC()
		}
	} else {
		occurredAt = occurredAt.UTC()
	}

	payload := &VideoProcessingFailed{
		VideoID:        video.VideoID,
		Status:         video.Status,
		MediaStatus:    video.MediaStatus,
		AnalysisStatus: video.AnalysisStatus,
		Stage:          stage,
		ErrorMessage:   errorMessage,
		JobID:          jobID,
		EmittedAt:      cloneTime(emittedAt),
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoProcessingFailed,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       VersionFromTime(occurredAt),
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}

// NewVideoVisibilityChangedEvent 构建可见性变更事件。
func NewVideoVisibilityChangedEvent(video *po.Video, previous po.VideoStatus, reason *string, eventID uuid.UUID, occurredAt time.Time) (*DomainEvent, error) {
	if video == nil {
		return nil, ErrNilVideo
	}
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}

	if occurredAt.IsZero() {
		if !video.UpdatedAt.IsZero() {
			occurredAt = video.UpdatedAt.UTC()
		} else {
			occurredAt = time.Now().UTC()
		}
	} else {
		occurredAt = occurredAt.UTC()
	}

	payload := &VideoVisibilityChanged{
		VideoID:          video.VideoID,
		Status:           video.Status,
		VisibilityStatus: video.VisibilityStatus,
		Reason:           reason,
	}
	if previous != "" {
		prev := previous
		payload.PreviousStatus = &prev
	}

	if video.PublishAt != nil {
		payload.PublishedAt = cloneTime(video.PublishAt)
	}

	event := &DomainEvent{
		EventID:       eventID,
		Kind:          KindVideoVisibilityChanged,
		AggregateID:   video.VideoID,
		AggregateType: AggregateTypeVideo,
		Version:       VersionFromTime(occurredAt),
		OccurredAt:    occurredAt,
		Payload:       payload,
	}
	return event, nil
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := t.UTC()
	return &c
}
