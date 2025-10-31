package outboxevents

import (
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
)

// ToProto 将领域事件转换为 protobuf Event。
func ToProto(evt *DomainEvent) (*videov1.Event, error) {
	if evt == nil {
		return nil, fmt.Errorf("events: nil domain event")
	}

	pb := &videov1.Event{
		EventId:       evt.EventID.String(),
		EventType:     kindToProto(evt.Kind),
		AggregateId:   evt.AggregateID.String(),
		AggregateType: evt.AggregateType,
		Version:       evt.Version,
		OccurredAt:    evt.OccurredAt.UTC().Format(time.RFC3339Nano),
	}

	switch payload := evt.Payload.(type) {
	case *VideoCreated:
		pb.Payload = &videov1.Event_Created{Created: encodeVideoCreated(evt, payload)}
	case *VideoUpdated:
		pb.Payload = &videov1.Event_Updated{Updated: encodeVideoUpdated(evt, payload)}
	case *VideoDeleted:
		pb.Payload = &videov1.Event_Deleted{Deleted: encodeVideoDeleted(evt, payload)}
	default:
		return nil, fmt.Errorf("events: unsupported payload type %T", payload)
	}

	return pb, nil
}

func encodeVideoCreated(evt *DomainEvent, payload *VideoCreated) *videov1.Event_VideoCreated {
	created := &videov1.Event_VideoCreated{
		VideoId:        payload.VideoID.String(),
		UploaderId:     payload.UploaderID.String(),
		Title:          payload.Title,
		Status:         payload.Status,
		MediaStatus:    payload.MediaStatus,
		AnalysisStatus: payload.AnalysisStatus,
		Version:        evt.Version,
		OccurredAt:     evt.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	if payload.Description != nil {
		created.Description = payload.Description
	}
	if payload.DurationMicros != nil {
		created.DurationMicros = payload.DurationMicros
	}
	if payload.PublishedAt != nil {
		publishedAt := payload.PublishedAt.UTC().Format(time.RFC3339Nano)
		created.PublishedAt = &publishedAt
	}
	return created
}

func encodeVideoUpdated(evt *DomainEvent, payload *VideoUpdated) *videov1.Event_VideoUpdated {
	updated := &videov1.Event_VideoUpdated{
		VideoId:    payload.VideoID.String(),
		Version:    evt.Version,
		OccurredAt: evt.OccurredAt.UTC().Format(time.RFC3339Nano),
		Tags:       payload.Tags,
	}
	if payload.Title != nil {
		updated.Title = payload.Title
	}
	if payload.Description != nil {
		updated.Description = payload.Description
	}
	if payload.Status != nil {
		updated.Status = payload.Status
	}
	if payload.MediaStatus != nil {
		updated.MediaStatus = payload.MediaStatus
	}
	if payload.AnalysisStatus != nil {
		updated.AnalysisStatus = payload.AnalysisStatus
	}
	if payload.DurationMicros != nil {
		updated.DurationMicros = payload.DurationMicros
	}
	if payload.ThumbnailURL != nil {
		updated.ThumbnailUrl = payload.ThumbnailURL
	}
	if payload.HLSMasterPlaylist != nil {
		updated.HlsMasterPlaylist = payload.HLSMasterPlaylist
	}
	if payload.Difficulty != nil {
		updated.Difficulty = payload.Difficulty
	}
	if payload.Summary != nil {
		updated.Summary = payload.Summary
	}
	if payload.RawSubtitleURL != nil {
		updated.RawSubtitleUrl = payload.RawSubtitleURL
	}
	if payload.VisibilityStatus != nil {
		updated.VisibilityStatus = payload.VisibilityStatus
	}
	if payload.PublishedAt != nil {
		publishedAt := payload.PublishedAt.UTC().Format(time.RFC3339Nano)
		updated.PublishedAt = &publishedAt
	}
	return updated
}

func encodeVideoDeleted(evt *DomainEvent, payload *VideoDeleted) *videov1.Event_VideoDeleted {
	deleted := &videov1.Event_VideoDeleted{
		VideoId:    payload.VideoID.String(),
		Version:    evt.Version,
		OccurredAt: evt.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	if payload.DeletedAt != nil {
		deletedAt := payload.DeletedAt.UTC().Format(time.RFC3339Nano)
		deleted.DeletedAt = &deletedAt
	}
	if payload.Reason != nil {
		deleted.Reason = payload.Reason
	}
	return deleted
}

func kindToProto(kind Kind) videov1.EventType {
	switch kind {
	case KindVideoCreated:
		return videov1.EventType_EVENT_TYPE_VIDEO_CREATED
	case KindVideoUpdated:
		return videov1.EventType_EVENT_TYPE_VIDEO_UPDATED
	case KindVideoDeleted:
		return videov1.EventType_EVENT_TYPE_VIDEO_DELETED
	default:
		return videov1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}
