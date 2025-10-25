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

// NewVideoUpdatedEvent 基于更新后的实体与变更集构建 VideoUpdated 事件。
func NewVideoUpdatedEvent(video *po.Video, changes VideoUpdateChanges, eventID uuid.UUID, occurredAt time.Time) (*videov1.Event, error) {
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

	version := VersionFromTime(occurredAt)
	payload := &videov1.VideoUpdated{
		VideoId:    video.VideoID.String(),
		Version:    version,
		OccurredAt: timestamppb.New(occurredAt.UTC()),
	}

	var hasChange bool

	if changes.Title != nil {
		payload.Title = wrapperspb.String(*changes.Title)
		hasChange = true
	}
	if changes.Description != nil {
		payload.Description = wrapperspb.String(*changes.Description)
		hasChange = true
	}
	if changes.Status != nil {
		value := string(*changes.Status)
		payload.Status = wrapperspb.String(value)
		hasChange = true
	}
	if changes.MediaStatus != nil {
		value := string(*changes.MediaStatus)
		payload.MediaStatus = wrapperspb.String(value)
		hasChange = true
	}
	if changes.AnalysisStatus != nil {
		value := string(*changes.AnalysisStatus)
		payload.AnalysisStatus = wrapperspb.String(value)
		hasChange = true
	}
	if changes.DurationMicros != nil {
		payload.DurationMicros = wrapperspb.Int64(*changes.DurationMicros)
		hasChange = true
	}
	if changes.ThumbnailURL != nil {
		payload.ThumbnailUrl = wrapperspb.String(*changes.ThumbnailURL)
		hasChange = true
	}
	if changes.HLSMasterPlaylist != nil {
		payload.HlsMasterPlaylist = wrapperspb.String(*changes.HLSMasterPlaylist)
		hasChange = true
	}
	if changes.Difficulty != nil {
		payload.Difficulty = wrapperspb.String(*changes.Difficulty)
		hasChange = true
	}
	if changes.Summary != nil {
		payload.Summary = wrapperspb.String(*changes.Summary)
		hasChange = true
	}
	if changes.RawSubtitleURL != nil {
		payload.RawSubtitleUrl = wrapperspb.String(*changes.RawSubtitleURL)
		hasChange = true
	}
	if !hasChange {
		return nil, ErrEmptyUpdatePayload
	}

	event := &videov1.Event{
		EventId:       eventID.String(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
		AggregateId:   video.VideoID.String(),
		AggregateType: AggregateTypeVideo,
		Version:       version,
		OccurredAt:    timestamppb.New(occurredAt.UTC()),
		Payload:       &videov1.Event_Updated{Updated: payload},
	}
	return event, nil
}
