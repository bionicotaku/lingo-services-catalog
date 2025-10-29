package services

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// LifecycleRepo 定义生命周期写入所需的持久化能力。
type LifecycleRepo interface {
	Create(ctx context.Context, sess txmanager.Session, input repositories.CreateVideoInput) (*po.Video, error)
	Update(ctx context.Context, sess txmanager.Session, input repositories.UpdateVideoInput) (*po.Video, error)
}

// LifecycleOutboxWriter 定义写 Outbox 的接口。
type LifecycleOutboxWriter interface {
	Enqueue(ctx context.Context, sess txmanager.Session, msg repositories.OutboxMessage) error
}

// LifecycleWriter 负责在事务内执行写模型操作并写入 Outbox。
type LifecycleWriter struct {
	repo      LifecycleRepo
	outbox    LifecycleOutboxWriter
	txManager txmanager.Manager
	log       *log.Helper
}

// NewLifecycleWriter 构造 LifecycleWriter。
func NewLifecycleWriter(repo LifecycleRepo, outbox LifecycleOutboxWriter, tx txmanager.Manager, logger log.Logger) *LifecycleWriter {
	return &LifecycleWriter{
		repo:      repo,
		outbox:    outbox,
		txManager: tx,
		log:       log.NewHelper(logger),
	}
}

type operationMetadata struct {
	IdempotencyKey string
}

// CreateVideoInput 表示创建视频的输入。
type CreateVideoInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
	IdempotencyKey   string
}

// UpdateVideoInput 表示更新视频的可选字段。
type UpdateVideoInput struct {
	VideoID           uuid.UUID
	Title             *string
	Description       *string
	Status            *po.VideoStatus
	MediaStatus       *po.StageStatus
	AnalysisStatus    *po.StageStatus
	RawFileSize       *int64
	RawResolution     *string
	RawBitrate        *int32
	DurationMicros    *int64
	EncodedResolution *string
	EncodedBitrate    *int32
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	Tags              []string
	VisibilityStatus  *string
	PublishAt         *time.Time
	RawSubtitleURL    *string
	ErrorMessage      *string
	MediaJobID        *string
	MediaEmittedAt    *time.Time
	AnalysisJobID     *string
	AnalysisEmittedAt *time.Time
	ExpectedVersion   *int64
	IdempotencyKey    string
}

// AdditionalEventBuilder 用于补充生命周期事件。
type AdditionalEventBuilder func(ctx context.Context, updated *po.Video, previous *po.Video) ([]*outboxevents.DomainEvent, error)

type updateVideoConfig struct {
	previous *po.Video
	builder  AdditionalEventBuilder
}

// UpdateVideoOption 配置 UpdateVideo 的扩展行为。
type UpdateVideoOption func(*updateVideoConfig)

// CreateVideo 创建视频记录并写入领域事件。
func (w *LifecycleWriter) CreateVideo(ctx context.Context, input CreateVideoInput) (*VideoRevision, error) {
	if input.UploadUserID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "upload_user_id is required")
	}
	if input.Title == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "title is required")
	}
	if input.RawFileReference == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "raw_file_reference is required")
	}

	meta := operationMetadata{IdempotencyKey: input.IdempotencyKey}

	var (
		created    *po.Video
		event      *outboxevents.DomainEvent
		occurredAt time.Time
	)

	err := w.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := w.repo.Create(txCtx, sess, repositories.CreateVideoInput{
			UploadUserID:     input.UploadUserID,
			Title:            input.Title,
			Description:      input.Description,
			RawFileReference: input.RawFileReference,
		})
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.CreatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		evtID := uuid.New()
		evt, buildErr := outboxevents.NewVideoCreatedEvent(video, evtID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}
		if err := w.enqueueOutbox(txCtx, sess, evt, occurredAt, meta); err != nil {
			return err
		}

		created = video
		event = evt
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			w.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}
		w.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	w.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s", created.VideoID, input.Title)
	return NewVideoRevision(created, event, occurredAt), nil
}

// UpdateVideo 执行部分更新并写入事件。
func (w *LifecycleWriter) UpdateVideo(ctx context.Context, input UpdateVideoInput, opts ...UpdateVideoOption) (*VideoRevision, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	if !hasUpdateFields(input) {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "no fields to update")
	}

	cfg := updateVideoConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if input.Status != nil && *input.Status == po.VideoStatusPublished {
		if input.PublishAt == nil && (cfg.previous == nil || cfg.previous.PublishAt == nil) {
			now := time.Now().UTC()
			input.PublishAt = &now
		}
		if input.VisibilityStatus == nil {
			if cfg.previous == nil || cfg.previous.VisibilityStatus != po.VisibilityPublic {
				visibility := po.VisibilityPublic
				input.VisibilityStatus = &visibility
			}
		}
	}

	if input.ExpectedVersion != nil && cfg.previous != nil && cfg.previous.Version != *input.ExpectedVersion {
		return nil, errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "version conflict")
	}

	meta := operationMetadata{IdempotencyKey: input.IdempotencyKey}

	var (
		updated     *po.Video
		updateEvent *outboxevents.DomainEvent
		occurredAt  time.Time
	)

	err := w.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := w.repo.Update(txCtx, sess, repositories.UpdateVideoInput{
			VideoID:           input.VideoID,
			Title:             input.Title,
			Description:       input.Description,
			Status:            input.Status,
			MediaStatus:       input.MediaStatus,
			AnalysisStatus:    input.AnalysisStatus,
			DurationMicros:    input.DurationMicros,
			EncodedResolution: input.EncodedResolution,
			EncodedBitrate:    input.EncodedBitrate,
			ThumbnailURL:      input.ThumbnailURL,
			HLSMasterPlaylist: input.HLSMasterPlaylist,
			Difficulty:        input.Difficulty,
			Summary:           input.Summary,
			Tags:              input.Tags,
			RawSubtitleURL:    input.RawSubtitleURL,
			ErrorMessage:      input.ErrorMessage,
			MediaJobID:        input.MediaJobID,
			MediaEmittedAt:    input.MediaEmittedAt,
			AnalysisJobID:     input.AnalysisJobID,
			AnalysisEmittedAt: input.AnalysisEmittedAt,
			RawFileSize:       input.RawFileSize,
			RawResolution:     input.RawResolution,
			RawBitrate:        input.RawBitrate,
			VisibilityStatus:  input.VisibilityStatus,
			PublishAt:         input.PublishAt,
		})
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.UpdatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}

		eventID := uuid.New()
		event, buildErr := outboxevents.NewVideoUpdatedEvent(video, outboxevents.VideoUpdateChanges{
			Title:             input.Title,
			Description:       input.Description,
			Status:            input.Status,
			MediaStatus:       input.MediaStatus,
			AnalysisStatus:    input.AnalysisStatus,
			DurationMicros:    input.DurationMicros,
			ThumbnailURL:      input.ThumbnailURL,
			HLSMasterPlaylist: input.HLSMasterPlaylist,
			Difficulty:        input.Difficulty,
			Summary:           input.Summary,
			VisibilityStatus:  input.VisibilityStatus,
			PublishAt:         input.PublishAt,
			RawSubtitleURL:    input.RawSubtitleURL,
		}, eventID, occurredAt)
		if buildErr != nil {
			if !stdErrors.Is(buildErr, outboxevents.ErrEmptyUpdatePayload) {
				return fmt.Errorf("build video updated event: %w", buildErr)
			}
		} else {
			if err := w.enqueueOutbox(txCtx, sess, event, occurredAt, meta); err != nil {
				return err
			}
			updateEvent = event
		}

		if cfg.builder != nil {
			extras, extraErr := cfg.builder(txCtx, video, cfg.previous)
			if extraErr != nil {
				return fmt.Errorf("build additional events: %w", extraErr)
			}
			for _, evt := range extras {
				if evt == nil {
					continue
				}
				if err := w.enqueueOutbox(txCtx, sess, evt, evt.OccurredAt, meta); err != nil {
					return err
				}
			}
		}

		updated = video
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			w.log.WithContext(ctx).Warnf("update video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "update timeout")
		}
		w.log.WithContext(ctx).Errorf("update video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to update video").WithCause(fmt.Errorf("update video: %w", err))
	}

	w.log.WithContext(ctx).Infof("UpdateVideo: video_id=%s", updated.VideoID)
	return NewVideoRevision(updated, updateEvent, occurredAt), nil
}

// WithPreviousVideo 将更新前的视频实体传入 UpdateVideo。
func WithPreviousVideo(previous *po.Video) UpdateVideoOption {
	return func(cfg *updateVideoConfig) {
		cfg.previous = previous
	}
}

// WithAdditionalEvents 配置额外事件构建器。
func WithAdditionalEvents(builder AdditionalEventBuilder) UpdateVideoOption {
	return func(cfg *updateVideoConfig) {
		cfg.builder = builder
	}
}

func hasUpdateFields(input UpdateVideoInput) bool {
	return input.Title != nil ||
		input.Description != nil ||
		input.Status != nil ||
		input.MediaStatus != nil ||
		input.AnalysisStatus != nil ||
		input.RawFileSize != nil ||
		input.RawResolution != nil ||
		input.RawBitrate != nil ||
		input.DurationMicros != nil ||
		input.EncodedResolution != nil ||
		input.EncodedBitrate != nil ||
		input.ThumbnailURL != nil ||
		input.HLSMasterPlaylist != nil ||
		input.Difficulty != nil ||
		input.Summary != nil ||
		len(input.Tags) > 0 ||
		input.RawSubtitleURL != nil ||
		input.ErrorMessage != nil ||
		input.MediaJobID != nil ||
		input.MediaEmittedAt != nil ||
		input.AnalysisJobID != nil ||
		input.AnalysisEmittedAt != nil ||
		input.VisibilityStatus != nil ||
		input.PublishAt != nil
}

func (w *LifecycleWriter) enqueueOutbox(ctx context.Context, sess txmanager.Session, event *outboxevents.DomainEvent, availableAt time.Time, meta operationMetadata) error {
	protoEvent, encodeErr := outboxevents.ToProto(event)
	if encodeErr != nil {
		return fmt.Errorf("convert event to proto: %w", encodeErr)
	}
	payload, marshalErr := proto.Marshal(protoEvent)
	if marshalErr != nil {
		return fmt.Errorf("marshal video event: %w", marshalErr)
	}

	attributes := outboxevents.BuildAttributes(event, outboxevents.SchemaVersionV1, outboxevents.TraceIDFromContext(ctx))
	if meta.IdempotencyKey != "" {
		attributes["idempotency_key"] = meta.IdempotencyKey
	}

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
	if err := w.outbox.Enqueue(ctx, sess, msg); err != nil {
		return fmt.Errorf("enqueue outbox: %w", err)
	}
	return nil
}
