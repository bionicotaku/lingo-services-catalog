package services

import (
	"context"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(videov1.ErrorReason_ERROR_REASON_VIDEO_NOT_FOUND.String(), "video not found")

// VideoLookupRepo 抽象出读取单个视频实体所需的仓储能力。
type VideoLookupRepo interface {
	GetLifecycleSnapshot(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error)
}

// VideoRevision 表示视频写操作后的最新快照及事件元数据。
type VideoRevision struct {
	VideoID        uuid.UUID
	Status         po.VideoStatus
	MediaStatus    po.StageStatus
	AnalysisStatus po.StageStatus
	Version        int64
	UpdatedAt      time.Time
	EventID        uuid.UUID
	OccurredAt     time.Time
}

// NewVideoRevision 根据领域实体与事件构造 VideoRevision。
func NewVideoRevision(video *po.Video, event *outboxevents.DomainEvent, occurredAt time.Time) *VideoRevision {
	if video == nil {
		return nil
	}
	if event != nil && occurredAt.IsZero() {
		occurredAt = event.OccurredAt
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	return &VideoRevision{
		VideoID:        video.VideoID,
		Status:         video.Status,
		MediaStatus:    video.MediaStatus,
		AnalysisStatus: video.AnalysisStatus,
		Version:        video.Version,
		UpdatedAt:      video.UpdatedAt.UTC(),
		EventID: func() uuid.UUID {
			if event != nil {
				return event.EventID
			}
			return uuid.Nil
		}(),
		OccurredAt: occurredAt.UTC(),
	}
}
