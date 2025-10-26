package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	outboxevents "github.com/bionicotaku/kratos-template/internal/models/outbox_events"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
	"github.com/bionicotaku/kratos-template/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// VideoCommandRepo 定义写模型需要的持久化行为。
type VideoCommandRepo interface {
	Create(ctx context.Context, sess txmanager.Session, input repositories.CreateVideoInput) (*po.Video, error)
	Update(ctx context.Context, sess txmanager.Session, input repositories.UpdateVideoInput) (*po.Video, error)
	Delete(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error)
}

// VideoOutboxWriter 定义 Outbox 写入行为。
type VideoOutboxWriter interface {
	Enqueue(ctx context.Context, sess txmanager.Session, msg repositories.OutboxMessage) error
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

// VideoCommandService 封装 Video 写模型用例。
type VideoCommandService struct {
	repo      VideoCommandRepo
	outbox    VideoOutboxWriter
	txManager txmanager.Manager
	log       *log.Helper
}

// NewVideoCommandService 构造一个 Video 写模型服务。
func NewVideoCommandService(repo VideoCommandRepo, outbox VideoOutboxWriter, tx txmanager.Manager, logger log.Logger) *VideoCommandService {
	return &VideoCommandService{
		repo:      repo,
		outbox:    outbox,
		txManager: tx,
		log:       log.NewHelper(logger),
	}
}

// CreateVideo 创建新视频记录。
func (s *VideoCommandService) CreateVideo(ctx context.Context, input CreateVideoInput) (*vo.VideoCreated, error) {
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	var created *po.Video
	var createdEvent *outboxevents.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err := s.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := s.repo.Create(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.CreatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		eventID = uuid.New()
		event, buildErr := outboxevents.NewVideoCreatedEvent(video, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}

		if err := s.enqueueOutbox(txCtx, sess, event, occurredAt); err != nil {
			return err
		}

		created = video
		createdEvent = event
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}
		s.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	s.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s status=%s", created.VideoID, created.Title, created.Status)
	return vo.NewVideoCreated(created, eventID, createdEvent.Version, occurredAt), nil
}

// UpdateVideo 更新视频元数据并写入 Outbox。
func (s *VideoCommandService) UpdateVideo(ctx context.Context, input UpdateVideoInput) (*vo.VideoUpdated, error) {
	if !hasUpdateFields(input) {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "no fields to update")
	}
	if input.DurationMicros != nil && *input.DurationMicros < 0 {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "duration_micros must be non-negative")
	}

	videoStatus, err := parseVideoStatus(input.Status)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), err.Error())
	}
	mediaStatus, err := parseStageStatus(input.MediaStatus)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), err.Error())
	}
	analysisStatus, err := parseStageStatus(input.AnalysisStatus)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), err.Error())
	}

	repoInput := repositories.UpdateVideoInput{
		VideoID:           input.VideoID,
		Title:             input.Title,
		Description:       input.Description,
		Status:            videoStatus,
		MediaStatus:       mediaStatus,
		AnalysisStatus:    analysisStatus,
		DurationMicros:    input.DurationMicros,
		ThumbnailURL:      input.ThumbnailURL,
		HLSMasterPlaylist: input.HLSMasterPlaylist,
		Difficulty:        input.Difficulty,
		Summary:           input.Summary,
		RawSubtitleURL:    input.RawSubtitleURL,
		ErrorMessage:      input.ErrorMessage,
	}

	var updated *po.Video
	var updateEvent *outboxevents.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err = s.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := s.repo.Update(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.UpdatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}

		eventID = uuid.New()
		changes := outboxevents.VideoUpdateChanges{
			Title:             input.Title,
			Description:       input.Description,
			Status:            videoStatus,
			MediaStatus:       mediaStatus,
			AnalysisStatus:    analysisStatus,
			DurationMicros:    input.DurationMicros,
			ThumbnailURL:      input.ThumbnailURL,
			HLSMasterPlaylist: input.HLSMasterPlaylist,
			Difficulty:        input.Difficulty,
			Summary:           input.Summary,
			RawSubtitleURL:    input.RawSubtitleURL,
		}

		event, buildErr := outboxevents.NewVideoUpdatedEvent(video, changes, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video updated event: %w", buildErr)
		}

		if err := s.enqueueOutbox(txCtx, sess, event, occurredAt); err != nil {
			return err
		}

		updated = video
		updateEvent = event
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("update video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "update timeout")
		}
		s.log.WithContext(ctx).Errorf("update video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to update video").WithCause(fmt.Errorf("update video: %w", err))
	}

	s.log.WithContext(ctx).Infof("UpdateVideo: video_id=%s", updated.VideoID)
	return vo.NewVideoUpdated(updated, eventID, updateEvent.Version, occurredAt), nil
}

// DeleteVideo 删除视频记录并写入删除事件。
func (s *VideoCommandService) DeleteVideo(ctx context.Context, input DeleteVideoInput) (*vo.VideoDeleted, error) {
	var deleted *po.Video
	var event *outboxevents.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err := s.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := s.repo.Delete(txCtx, sess, input.VideoID)
		if repoErr != nil {
			return repoErr
		}
		deleted = video

		occurredAt = time.Now().UTC()
		eventID = uuid.New()
		delEvent, buildErr := outboxevents.NewVideoDeletedEvent(video, eventID, occurredAt, input.Reason)
		if buildErr != nil {
			return fmt.Errorf("build video deleted event: %w", buildErr)
		}
		event = delEvent

		if err := s.enqueueOutbox(txCtx, sess, delEvent, occurredAt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("delete video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "delete timeout")
		}
		s.log.WithContext(ctx).Errorf("delete video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to delete video").WithCause(fmt.Errorf("delete video: %w", err))
	}

	s.log.WithContext(ctx).Infof("DeleteVideo: video_id=%s", deleted.VideoID)
	return vo.NewVideoDeleted(deleted.VideoID, occurredAt, eventID, event.Version, occurredAt), nil
}

// enqueueOutbox 将领域事件写入 Outbox。
func (s *VideoCommandService) enqueueOutbox(ctx context.Context, sess txmanager.Session, event *outboxevents.DomainEvent, availableAt time.Time) error {
	protoEvent, encodeErr := outboxevents.ToProto(event)
	if encodeErr != nil {
		return fmt.Errorf("convert event to proto: %w", encodeErr)
	}
	payload, marshalErr := proto.Marshal(protoEvent)
	if marshalErr != nil {
		return fmt.Errorf("marshal video event: %w", marshalErr)
	}

	attributes := outboxevents.BuildAttributes(event, outboxevents.SchemaVersionV1, outboxevents.TraceIDFromContext(ctx))
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	msg := repositories.OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     outboxevents.FormatEventType(event.Kind),
		Payload:       payload,
		Headers:       attributes,
		AvailableAt:   availableAt,
	}
	if err := s.outbox.Enqueue(ctx, sess, msg); err != nil {
		return fmt.Errorf("enqueue outbox: %w", err)
	}
	return nil
}

func hasUpdateFields(input UpdateVideoInput) bool {
	return input.Title != nil ||
		input.Description != nil ||
		input.Status != nil ||
		input.MediaStatus != nil ||
		input.AnalysisStatus != nil ||
		input.DurationMicros != nil ||
		input.ThumbnailURL != nil ||
		input.HLSMasterPlaylist != nil ||
		input.Difficulty != nil ||
		input.Summary != nil ||
		input.RawSubtitleURL != nil ||
		input.ErrorMessage != nil
}

func parseVideoStatus(status *string) (*po.VideoStatus, error) {
	if status == nil {
		return nil, nil
	}
	value := strings.TrimSpace(strings.ToLower(*status))
	if value == "" {
		return nil, nil
	}
	enum := po.VideoStatus(value)
	switch enum {
	case po.VideoStatusPendingUpload,
		po.VideoStatusProcessing,
		po.VideoStatusReady,
		po.VideoStatusPublished,
		po.VideoStatusFailed,
		po.VideoStatusRejected,
		po.VideoStatusArchived:
		return &enum, nil
	default:
		return nil, fmt.Errorf("invalid video status: %s", value)
	}
}

func parseStageStatus(status *string) (*po.StageStatus, error) {
	if status == nil {
		return nil, nil
	}
	value := strings.TrimSpace(strings.ToLower(*status))
	if value == "" {
		return nil, nil
	}
	enum := po.StageStatus(value)
	switch enum {
	case po.StagePending, po.StageProcessing, po.StageReady, po.StageFailed:
		return &enum, nil
	default:
		return nil, fmt.Errorf("invalid stage status: %s", value)
	}
}
