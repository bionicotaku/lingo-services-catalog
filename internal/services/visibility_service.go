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

// UpdateVisibilityAction 表示目标可见性操作。
type UpdateVisibilityAction string

const (
	// VisibilityPublish 表示将视频发布为公开状态。
	VisibilityPublish UpdateVisibilityAction = "publish"
	// VisibilityReject 表示拒绝或下架视频。
	VisibilityReject UpdateVisibilityAction = "reject"
	// VisibilityArchive 表示归档视频。
	VisibilityArchive UpdateVisibilityAction = "archive"
)

// UpdateVisibilityInput 描述可见性变更所需字段。
type UpdateVisibilityInput struct {
	VideoID         uuid.UUID
	Action          UpdateVisibilityAction
	Reason          *string
	ExpectedVersion *int64
	IdempotencyKey  string
}

// VisibilityService 负责发布/拒绝/归档视频。
type VisibilityService struct {
	writer *LifecycleWriter
	repo   VideoLookupRepo
}

// NewVisibilityService 构造 VisibilityService。
func NewVisibilityService(writer *LifecycleWriter, repo VideoLookupRepo) *VisibilityService {
	return &VisibilityService{writer: writer, repo: repo}
}

// UpdateVisibility 执行可见性变更。
func (s *VisibilityService) UpdateVisibility(ctx context.Context, input UpdateVisibilityInput) (*VideoRevision, error) {
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

	var targetStatus po.VideoStatus
	var visibilityStatus *string
	switch input.Action {
	case VisibilityPublish:
		targetStatus = po.VideoStatusPublished
		if current.MediaStatus != po.StageReady || current.AnalysisStatus != po.StageReady {
			return nil, errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video not ready for publish")
		}
		if current.VisibilityStatus != po.VisibilityPublic {
			value := po.VisibilityPublic
			visibilityStatus = &value
		}
	case VisibilityReject:
		targetStatus = po.VideoStatusRejected
		if current.VisibilityStatus != po.VisibilityPrivate {
			value := po.VisibilityPrivate
			visibilityStatus = &value
		}
	case VisibilityArchive:
		targetStatus = po.VideoStatusArchived
		if current.VisibilityStatus != po.VisibilityPrivate {
			value := po.VisibilityPrivate
			visibilityStatus = &value
		}
	default:
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "unknown visibility action")
	}

	if current.Status == targetStatus {
		return nil, errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "status already applied")
	}

	statusValue := targetStatus
	updateInput := UpdateVideoInput{
		VideoID:          input.VideoID,
		Status:           &statusValue,
		ErrorMessage:     input.Reason,
		VisibilityStatus: visibilityStatus,
	}
	updateInput.IdempotencyKey = input.IdempotencyKey
	updateInput.ExpectedVersion = input.ExpectedVersion

	return s.writer.UpdateVideo(
		ctx,
		updateInput,
		WithPreviousVideo(current),
		WithAdditionalEvents(func(_ context.Context, updated *po.Video, previous *po.Video) ([]*outboxevents.DomainEvent, error) {
			if previous == nil {
				return nil, nil
			}
			if previous.Status == updated.Status {
				return nil, nil
			}
			event, err := outboxevents.NewVideoVisibilityChangedEvent(
				updated,
				previous.Status,
				input.Reason,
				uuid.New(),
				visibilityOccurredAt(updated),
			)
			if err != nil {
				return nil, err
			}
			return []*outboxevents.DomainEvent{event}, nil
		}),
	)
}

func visibilityOccurredAt(video *po.Video) time.Time {
	if video != nil && !video.UpdatedAt.IsZero() {
		return video.UpdatedAt.UTC()
	}
	return time.Time{}
}
