package services

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// UpdateOriginalMediaInput 描述原始媒体属性写入所需字段。
type UpdateOriginalMediaInput struct {
	VideoID         uuid.UUID
	RawFileSize     *int64
	RawResolution   *string
	RawBitrate      *int32
	ExpectedVersion *int64
	IdempotencyKey  string
}

type originalMediaRepo interface {
	GetLifecycleSnapshot(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error)
}

// OriginalMediaService 负责记录上传完成后的原始媒体属性。
type OriginalMediaService struct {
	writer *LifecycleWriter
	repo   originalMediaRepo
}

// NewOriginalMediaService 构造 OriginalMediaService。
func NewOriginalMediaService(writer *LifecycleWriter, repo originalMediaRepo) *OriginalMediaService {
	return &OriginalMediaService{writer: writer, repo: repo}
}

// UpdateOriginalMedia 写入原始媒体属性。
func (s *OriginalMediaService) UpdateOriginalMedia(ctx context.Context, input UpdateOriginalMediaInput) (*VideoRevision, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	if input.RawFileSize == nil && input.RawResolution == nil && input.RawBitrate == nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "no media fields provided")
	}

	current, err := s.repo.GetLifecycleSnapshot(ctx, nil, input.VideoID)
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("load video: %v", err))
	}

	updateInput := UpdateVideoInput{
		VideoID:         input.VideoID,
		RawFileSize:     input.RawFileSize,
		RawResolution:   input.RawResolution,
		RawBitrate:      input.RawBitrate,
		IdempotencyKey:  input.IdempotencyKey,
		ExpectedVersion: input.ExpectedVersion,
	}

	return s.writer.UpdateVideo(ctx, updateInput, WithPreviousVideo(current))
}
