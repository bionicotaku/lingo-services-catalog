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
	case *VideoMediaReady:
		pb.Payload = &videov1.Event_MediaReady{MediaReady: encodeVideoMediaReady(evt, payload)}
	case *VideoAIEnriched:
		pb.Payload = &videov1.Event_AiEnriched{AiEnriched: encodeVideoAIEnriched(evt, payload)}
	case *VideoProcessingFailed:
		pb.Payload = &videov1.Event_ProcessingFailed{ProcessingFailed: encodeVideoProcessingFailed(evt, payload)}
	case *VideoVisibilityChanged:
		pb.Payload = &videov1.Event_VisibilityChanged{VisibilityChanged: encodeVideoVisibilityChanged(evt, payload)}
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

func encodeVideoMediaReady(evt *DomainEvent, payload *VideoMediaReady) *videov1.Event_VideoMediaReady {
	media := &videov1.Event_VideoMediaReady{
		VideoId:           payload.VideoID.String(),
		Version:           evt.Version,
		OccurredAt:        evt.OccurredAt.UTC().Format(time.RFC3339Nano),
		Status:            string(payload.Status),
		MediaStatus:       string(payload.MediaStatus),
		AnalysisStatus:    string(payload.AnalysisStatus),
		DurationMicros:    payload.DurationMicros,
		EncodedResolution: payload.EncodedResolution,
		EncodedBitrate:    payload.EncodedBitrate,
		ThumbnailUrl:      payload.ThumbnailURL,
		HlsMasterPlaylist: payload.HLSMasterPlaylist,
		JobId:             payload.JobID,
	}
	if payload.EmittedAt != nil {
		media.EmittedAt = toProtoTime(payload.EmittedAt)
	}
	return media
}

func encodeVideoAIEnriched(evt *DomainEvent, payload *VideoAIEnriched) *videov1.Event_VideoAIEnriched {
	enriched := &videov1.Event_VideoAIEnriched{
		VideoId:        payload.VideoID.String(),
		Version:        evt.Version,
		OccurredAt:     evt.OccurredAt.UTC().Format(time.RFC3339Nano),
		Status:         string(payload.Status),
		AnalysisStatus: string(payload.AnalysisStatus),
		MediaStatus:    string(payload.MediaStatus),
		Difficulty:     payload.Difficulty,
		Summary:        payload.Summary,
		Tags:           append([]string(nil), payload.Tags...),
		RawSubtitleUrl: payload.RawSubtitleURL,
		JobId:          payload.JobID,
		ErrorMessage:   payload.ErrorMessage,
	}
	if payload.EmittedAt != nil {
		enriched.EmittedAt = toProtoTime(payload.EmittedAt)
	}
	return enriched
}

func encodeVideoProcessingFailed(evt *DomainEvent, payload *VideoProcessingFailed) *videov1.Event_VideoProcessingFailed {
	failed := &videov1.Event_VideoProcessingFailed{
		VideoId:        payload.VideoID.String(),
		Version:        evt.Version,
		OccurredAt:     evt.OccurredAt.UTC().Format(time.RFC3339Nano),
		FailedStage:    payload.Stage,
		ErrorMessage:   payload.ErrorMessage,
		JobId:          payload.JobID,
		Status:         string(payload.Status),
		MediaStatus:    string(payload.MediaStatus),
		AnalysisStatus: string(payload.AnalysisStatus),
	}
	if payload.EmittedAt != nil {
		failed.EmittedAt = toProtoTime(payload.EmittedAt)
	}
	return failed
}

func encodeVideoVisibilityChanged(evt *DomainEvent, payload *VideoVisibilityChanged) *videov1.Event_VideoVisibilityChanged {
	visibility := &videov1.Event_VideoVisibilityChanged{
		VideoId:    payload.VideoID.String(),
		Version:    evt.Version,
		OccurredAt: evt.OccurredAt.UTC().Format(time.RFC3339Nano),
		Status:     string(payload.Status),
		Reason:     payload.Reason,
	}
	if payload.VisibilityStatus != "" {
		value := payload.VisibilityStatus
		visibility.VisibilityStatus = &value
	}
	if payload.PreviousStatus != nil {
		value := string(*payload.PreviousStatus)
		visibility.PreviousStatus = &value
	}
	if payload.PublishedAt != nil {
		visibility.PublishedAt = toProtoTime(payload.PublishedAt)
	}
	return visibility
}

func kindToProto(kind Kind) videov1.EventType {
	switch kind {
	case KindVideoCreated:
		return videov1.EventType_EVENT_TYPE_VIDEO_CREATED
	case KindVideoUpdated:
		return videov1.EventType_EVENT_TYPE_VIDEO_UPDATED
	case KindVideoDeleted:
		return videov1.EventType_EVENT_TYPE_VIDEO_DELETED
	case KindVideoMediaReady:
		return videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY
	case KindVideoAIEnriched:
		return videov1.EventType_EVENT_TYPE_VIDEO_AI_ENRICHED
	case KindVideoProcessingFailed:
		return videov1.EventType_EVENT_TYPE_VIDEO_PROCESSING_FAILED
	case KindVideoVisibilityChanged:
		return videov1.EventType_EVENT_TYPE_VIDEO_VISIBILITY_CHANGED
	default:
		return videov1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}

func toProtoTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.UTC().Format(time.RFC3339Nano)
	return &v
}
