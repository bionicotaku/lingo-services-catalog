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

// UpdateVisibilityAction 表示目标可见性操作。
type UpdateVisibilityAction string

const (
	VisibilityPublish UpdateVisibilityAction = "publish"
	VisibilityReject  UpdateVisibilityAction = "reject"
	VisibilityArchive UpdateVisibilityAction = "archive"
)

// UpdateVisibilityInput 描述可见性变更所需字段。
type UpdateVisibilityInput struct {
	VideoID uuid.UUID
	Action  UpdateVisibilityAction
	Reason  *string
}

// VisibilityService 负责发布/拒绝/归档视频。
type VisibilityService struct {
	commands *VideoCommandService
	repo     *repositories.VideoRepository
}

// NewVisibilityService 构造 VisibilityService。
func NewVisibilityService(commands *VideoCommandService, repo *repositories.VideoRepository) *VisibilityService {
	return &VisibilityService{commands: commands, repo: repo}
}

// UpdateVisibility 执行可见性变更。
func (s *VisibilityService) UpdateVisibility(ctx context.Context, input UpdateVisibilityInput) (*vo.VideoUpdated, error) {
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

	var targetStatus po.VideoStatus
	switch input.Action {
	case VisibilityPublish:
		targetStatus = po.VideoStatusPublished
		if current.MediaStatus != po.StageReady || current.AnalysisStatus != po.StageReady {
			return nil, errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video not ready for publish")
		}
	case VisibilityReject:
		targetStatus = po.VideoStatusRejected
	case VisibilityArchive:
		targetStatus = po.VideoStatusArchived
	default:
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "unknown visibility action")
	}

	if current.Status == targetStatus {
		return nil, errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "status already applied")
	}

	statusValue := string(targetStatus)
	updateInput := UpdateVideoInput{
		VideoID:      input.VideoID,
		Status:       &statusValue,
		ErrorMessage: input.Reason,
	}

	return s.commands.UpdateVideo(ctx, updateInput)
}
