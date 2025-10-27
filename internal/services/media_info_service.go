package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// UpdateMediaInfoInput 描述转码产物写入所需字段。
type UpdateMediaInfoInput struct {
	VideoID           uuid.UUID
	DurationMicros    *int64
	EncodedResolution *string
	EncodedBitrate    *int32
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	MediaStatus       *po.StageStatus
	JobID             string
	EmittedAt         time.Time
	ExpectedVersion   *int64
	IdempotencyKey    string
}

// MediaInfoService 负责更新媒体产物并重算总体状态。
type MediaInfoService struct {
	writer *LifecycleWriter
	repo   VideoLookupRepo
}

// NewMediaInfoService 构造 MediaInfoService。
func NewMediaInfoService(writer *LifecycleWriter, repo VideoLookupRepo) *MediaInfoService {
	return &MediaInfoService{writer: writer, repo: repo}
}

// UpdateMediaInfo 写入媒体产物并按需推进媒体阶段状态。
func (s *MediaInfoService) UpdateMediaInfo(ctx context.Context, input UpdateMediaInfoInput) (*VideoRevision, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	current, err := s.repo.GetLifecycleSnapshot(ctx, nil, input.VideoID)
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("load video: %v", err))
	}

	mediaStatus := current.MediaStatus
	if input.MediaStatus != nil {
		mediaStatus = *input.MediaStatus
	}
	analysisStatus := current.AnalysisStatus

	updateInput := UpdateVideoInput{
		VideoID:           input.VideoID,
		DurationMicros:    input.DurationMicros,
		EncodedResolution: input.EncodedResolution,
		EncodedBitrate:    input.EncodedBitrate,
		ThumbnailURL:      input.ThumbnailURL,
		HLSMasterPlaylist: input.HLSMasterPlaylist,
	}
	if input.MediaStatus != nil {
		status := *input.MediaStatus
		updateInput.MediaStatus = &status
	}
	if input.JobID != "" {
		job := input.JobID
		updateInput.MediaJobID = &job
	}
	if !input.EmittedAt.IsZero() {
		emitted := input.EmittedAt.UTC()
		updateInput.MediaEmittedAt = &emitted
	}
	updateInput.IdempotencyKey = input.IdempotencyKey
	updateInput.ExpectedVersion = input.ExpectedVersion

	computed := computeOverallStatus(current.Status, mediaStatus, analysisStatus, mediaStatus)
	if computed != current.Status {
		statusValue := computed
		updateInput.Status = &statusValue
	}

	return s.writer.UpdateVideo(
		ctx,
		updateInput,
		WithPreviousVideo(current),
		WithAdditionalEvents(func(_ context.Context, updated *po.Video, previous *po.Video) ([]*outboxevents.DomainEvent, error) {
			if previous == nil {
				return nil, nil
			}
			if updated.MediaStatus != po.StageReady {
				return nil, nil
			}
			if previous.MediaStatus == po.StageReady && !mediaPayloadChanged(previous, updated) {
				return nil, nil
			}
			event, err := outboxevents.NewVideoMediaReadyEvent(updated, uuid.New(), mediaOccurredAt(updated))
			if err != nil {
				return nil, err
			}
			return []*outboxevents.DomainEvent{event}, nil
		}),
	)
}

func mediaOccurredAt(video *po.Video) time.Time {
	if video == nil || video.MediaEmittedAt == nil {
		return time.Time{}
	}
	return video.MediaEmittedAt.UTC()
}

func mediaPayloadChanged(previous, updated *po.Video) bool {
	if previous == nil || updated == nil {
		return true
	}
	switch {
	case !equalStringPtr(previous.EncodedResolution, updated.EncodedResolution):
		return true
	case !equalIntPtr(previous.EncodedBitrate, updated.EncodedBitrate):
		return true
	case !equalStringPtr(previous.ThumbnailURL, updated.ThumbnailURL):
		return true
	case !equalStringPtr(previous.HLSMasterPlaylist, updated.HLSMasterPlaylist):
		return true
	case !equalInt64Ptr(previous.DurationMicros, updated.DurationMicros):
		return true
	default:
		return false
	}
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalIntPtr(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalInt64Ptr(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
