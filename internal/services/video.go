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
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	var created *po.Video
	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := uc.repo.Create(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		occurredAt := time.Now().UTC()
		eventID := uuid.New()
		event, buildErr := events.NewVideoCreatedEvent(video, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}

		payload, marshalErr := proto.Marshal(event)
		if marshalErr != nil {
			return fmt.Errorf("marshal video event: %w", marshalErr)
		}

		headersMap := events.BuildAttributes(event, events.SchemaVersionV1, events.TraceIDFromContext(txCtx))
		headers, hdrErr := events.MarshalAttributes(headersMap)
		if hdrErr != nil {
			return fmt.Errorf("encode event headers: %w", hdrErr)
		}

		msg := repositories.OutboxMessage{
			EventID:       eventID,
			AggregateType: events.AggregateTypeVideo,
			AggregateID:   video.VideoID,
			EventType:     events.FormatEventType(event.GetEventType()),
			Payload:       payload,
			Headers:       headers,
			AvailableAt:   occurredAt,
		}
		if err := uc.outboxRepo.Enqueue(txCtx, sess, msg); err != nil {
			return fmt.Errorf("enqueue outbox: %w", err)
		}

		created = video
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}
		uc.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s status=%s", created.VideoID, created.Title, created.Status)
	return vo.NewVideoCreated(created), nil
}

// GetVideoDetail 查询视频详情。
func (uc *VideoUsecase) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	var videoView *po.VideoReadyView
	err := uc.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		videoView, repoErr = uc.repo.FindByID(txCtx, sess, videoID)
		return repoErr
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		uc.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	uc.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", videoView.VideoID, videoView.Status)
	return vo.NewVideoDetail(videoView), nil
}
