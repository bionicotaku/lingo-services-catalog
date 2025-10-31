package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
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
	)
}
