package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
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
}

// MediaInfoService 负责更新媒体产物并重算总体状态。
type MediaInfoService struct {
	commands *VideoCommandService
	repo     *repositories.VideoRepository
}

// NewMediaInfoService 构造 MediaInfoService。
func NewMediaInfoService(commands *VideoCommandService, repo *repositories.VideoRepository) *MediaInfoService {
	return &MediaInfoService{commands: commands, repo: repo}
}

// UpdateMediaInfo 写入媒体产物并按需推进媒体阶段状态。
func (s *MediaInfoService) UpdateMediaInfo(ctx context.Context, input UpdateMediaInfoInput) (*vo.VideoUpdated, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	current, err := s.repo.GetByID(ctx, nil, input.VideoID)
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
		statusValue := string(*input.MediaStatus)
		updateInput.MediaStatus = &statusValue
	}

	computed := computeOverallStatus(current.Status, mediaStatus, analysisStatus, mediaStatus)
	if computed != current.Status {
		statusValue := string(computed)
		updateInput.Status = &statusValue
	}

	return s.commands.UpdateVideo(
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
