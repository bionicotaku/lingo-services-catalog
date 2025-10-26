package services

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
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

	return s.commands.UpdateVideo(ctx, updateInput)
}
