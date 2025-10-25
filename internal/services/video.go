package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/events"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
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

// CreateVideo 创建新视频记录（对应 VideoCommandService.CreateVideo RPC）。
func (uc *VideoUsecase) CreateVideo(ctx context.Context, input CreateVideoInput) (*vo.VideoCreated, error) {
	// 将业务输入转换为仓储层使用的结构体，避免直接依赖外部 DTO。
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	var created *po.Video
	// 使用 TxManager 确保“写视频 + 写 Outbox”在同一数据库事务中完成。
	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		// 先写入核心业务表，生成视频记录及主键。
		video, repoErr := uc.repo.Create(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		// 构建事件元数据，使用统一的事件生成器保持 schema 一致。
		occurredAt := time.Now().UTC()
		eventID := uuid.New()
		event, buildErr := events.NewVideoCreatedEvent(video, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}

		// 序列化事件载荷，后续由 Outbox Publisher 透传至 Kafka/GCS 等下游。
		payload, marshalErr := proto.Marshal(event)
		if marshalErr != nil {
			return fmt.Errorf("marshal video event: %w", marshalErr)
		}

		// 根据事件内容构建头信息（schema 版本、trace ID 等）以便消费者过滤。
		headersMap := events.BuildAttributes(event, events.SchemaVersionV1, events.TraceIDFromContext(txCtx))
		headers, hdrErr := events.MarshalAttributes(headersMap)
		if hdrErr != nil {
			return fmt.Errorf("encode event headers: %w", hdrErr)
		}

		// 将事件封装成 Outbox 消息，准备与业务写入一同提交。
		msg := repositories.OutboxMessage{
			EventID:       eventID,
			AggregateType: events.AggregateTypeVideo,
			AggregateID:   video.VideoID,
			EventType:     events.FormatEventType(event.GetEventType()),
			Payload:       payload,
			Headers:       headers,
			AvailableAt:   occurredAt,
		}
		// 在相同事务中写入 Outbox，确保事件与数据状态一致。
		if err := uc.outboxRepo.Enqueue(txCtx, sess, msg); err != nil {
			return fmt.Errorf("enqueue outbox: %w", err)
		}

		// 将创建结果带出事务闭包，供返回层使用。
		created = video
		return nil
	})
	if err != nil {
		// 统一处理上下文超时，方便调用方区分失败原因。
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}
		// 其他错误记录详细日志并包装为 Problem Details。
		uc.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	// 打印结构化日志方便排障，同时返回 VO 给上层。
	uc.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s status=%s", created.VideoID, created.Title, created.Status)
	return vo.NewVideoCreated(created), nil
}

// GetVideoDetail 查询视频详情。
func (uc *VideoUsecase) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	// 使用只读事务包裹查询，未来可统一接入读写分离。
	var videoView *po.VideoReadyView
	err := uc.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		// 委托仓储层执行查询，并将结果赋值到外层变量。
		var repoErr error
		videoView, repoErr = uc.repo.FindByID(txCtx, sess, videoID)
		return repoErr
	})
	if err != nil {
		// 将仓储层的 NotFound 转译成对外哨兵错误，保持 Problem 语义一致。
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		// 读操作也需要区分超时，避免调用方误判。
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		// 其他错误统一包装成 Internal Server Problem。
		uc.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	// 为调试提供 Debug 日志，并将仓储返回的视图转换成 VO。
	uc.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", videoView.VideoID, videoView.Status)
	return vo.NewVideoDetail(videoView), nil
}
