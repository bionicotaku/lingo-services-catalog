package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/events"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(videov1.ErrorReason_ERROR_REASON_VIDEO_NOT_FOUND.String(), "video not found")

// VideoRepo 定义 Video 实体的持久化行为接口。
type VideoRepo interface {
	Create(ctx context.Context, sess txmanager.Session, input repositories.CreateVideoInput) (*po.Video, error)
	Update(ctx context.Context, sess txmanager.Session, input repositories.UpdateVideoInput) (*po.Video, error)
	Delete(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error)
	FindByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error)
}

// OutboxRepo 定义 Outbox 写入行为。
type OutboxRepo interface {
	Enqueue(ctx context.Context, sess txmanager.Session, msg repositories.OutboxMessage) error
}

// VideoUsecase 封装 Video 相关的业务用例逻辑。
type VideoUsecase struct {
	repo       VideoRepo
	outboxRepo OutboxRepo
	txManager  txmanager.Manager
	log        *log.Helper
}

// NewVideoUsecase 构造一个 Video 业务用例实例。
func NewVideoUsecase(repo VideoRepo, outbox OutboxRepo, tx txmanager.Manager, logger log.Logger) *VideoUsecase {
	uc := &VideoUsecase{
		repo:       repo,
		outboxRepo: outbox,
		txManager:  tx,
		log:        log.NewHelper(logger),
	}
	return uc
}

// CreateVideoInput 表示创建视频的输入参数。
type CreateVideoInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
}

// UpdateVideoInput 表示更新视频时的可选字段。
type UpdateVideoInput struct {
	VideoID           uuid.UUID
	Title             *string
	Description       *string
	Status            *string
	MediaStatus       *string
	AnalysisStatus    *string
	DurationMicros    *int64
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	RawSubtitleURL    *string
	ErrorMessage      *string
}

// DeleteVideoInput 表示删除视频时的输入。
type DeleteVideoInput struct {
	VideoID uuid.UUID
	Reason  *string
}

func (uc *VideoUsecase) enqueueOutbox(ctx context.Context, sess txmanager.Session, event *videov1.Event, eventID uuid.UUID, aggregateID uuid.UUID, availableAt time.Time) error {
	payload, marshalErr := proto.Marshal(event)
	if marshalErr != nil {
		return fmt.Errorf("marshal video event: %w", marshalErr)
	}

	headersMap := events.BuildAttributes(event, events.SchemaVersionV1, events.TraceIDFromContext(ctx))
	headers, hdrErr := events.MarshalAttributes(headersMap)
	if hdrErr != nil {
		return fmt.Errorf("encode event headers: %w", hdrErr)
	}

	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	msg := repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: events.AggregateTypeVideo,
		AggregateID:   aggregateID,
		EventType:     events.FormatEventType(event.GetEventType()),
		Payload:       payload,
		Headers:       headers,
		AvailableAt:   availableAt,
	}
	if err := uc.outboxRepo.Enqueue(ctx, sess, msg); err != nil {
		return fmt.Errorf("enqueue outbox: %w", err)
	}
	return nil
}
