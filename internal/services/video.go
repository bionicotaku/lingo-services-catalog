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

// enqueueOutbox 将领域事件写入 Outbox。
//
// 处理流程：
//  1. 通过 events.ToProto 将 DomainEvent 转换为 protobuf Event。
//  2. 序列化为字节数组，用作 Outbox payload。
//  3. 使用 events.BuildAttributes 生成消息属性（event_id、occurred_at、schema_version 等）。
//  4. 构造 repositories.OutboxMessage 并调用仓储入队；若 availableAt 未设置，则使用当前 UTC 时间。
//
// 返回：处理过程中如有错误，统一包装上下文并返回。
func (uc *VideoUsecase) enqueueOutbox(ctx context.Context, sess txmanager.Session, event *events.DomainEvent, availableAt time.Time) error {
	// 1) 将领域事件转换为 protobuf Event。
	protoEvent, encodeErr := events.ToProto(event)
	if encodeErr != nil {
		return fmt.Errorf("convert event to proto: %w", encodeErr)
	}

	// 2) 序列化为字节数组，用作 Outbox payload。
	payload, marshalErr := proto.Marshal(protoEvent)
	if marshalErr != nil {
		return fmt.Errorf("marshal video event: %w", marshalErr)
	}

	// 3) 生成消息属性（含 trace_id、schema_version 等）。
	attributes := events.BuildAttributes(event, events.SchemaVersionV1, events.TraceIDFromContext(ctx))

	// 4) availableAt 未设置时使用当前时间，便于调度排序。
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	// 5) 构造 Outbox 消息并入队。
	msg := repositories.OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     events.FormatEventType(event.Kind),
		Payload:       payload,
		Headers:       attributes,
		AvailableAt:   availableAt,
	}
	if err := uc.outboxRepo.Enqueue(ctx, sess, msg); err != nil {
		return fmt.Errorf("enqueue outbox: %w", err)
	}
	return nil
}
