package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/events"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
	"github.com/bionicotaku/kratos-template/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// CreateVideo 创建新视频记录（对应 VideoCommandService.CreateVideo RPC）。
func (uc *VideoUsecase) CreateVideo(ctx context.Context, input CreateVideoInput) (*vo.VideoCreated, error) {
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	var created *po.Video
	var createdEvent *videov1.Event
	var eventID uuid.UUID
	var occurredAt time.Time

	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := uc.repo.Create(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.CreatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		eventID = uuid.New()
		event, buildErr := events.NewVideoCreatedEvent(video, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}

		if err := uc.enqueueOutbox(txCtx, sess, event, eventID, video.VideoID, occurredAt); err != nil {
			return err
		}

		created = video
		createdEvent = event
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
	return vo.NewVideoCreated(created, eventID, createdEvent.GetVersion(), occurredAt), nil
}

// UpdateVideo 更新视频元数据并写入 Outbox。
func (uc *VideoUsecase) UpdateVideo(ctx context.Context, input UpdateVideoInput) (*vo.VideoUpdated, error) {
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
	var updateEvent *videov1.Event
	var eventID uuid.UUID
	var occurredAt time.Time

	err = uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := uc.repo.Update(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		occurredAt = video.UpdatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}

		eventID = uuid.New()
		changes := events.VideoUpdateChanges{
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

		event, buildErr := events.NewVideoUpdatedEvent(video, changes, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video updated event: %w", buildErr)
		}

		if err := uc.enqueueOutbox(txCtx, sess, event, eventID, video.VideoID, occurredAt); err != nil {
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
			uc.log.WithContext(ctx).Warnf("update video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "update timeout")
		}
		uc.log.WithContext(ctx).Errorf("update video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to update video").WithCause(fmt.Errorf("update video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("UpdateVideo: video_id=%s", updated.VideoID)
	return vo.NewVideoUpdated(updated, eventID, updateEvent.GetVersion(), occurredAt), nil
}

// DeleteVideo 删除视频并记录事件。
func (uc *VideoUsecase) DeleteVideo(ctx context.Context, input DeleteVideoInput) (*vo.VideoDeleted, error) {
	var deleted *po.Video
	var deleteEvent *videov1.Event
	var eventID uuid.UUID
	var occurredAt time.Time

	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		video, repoErr := uc.repo.Delete(txCtx, sess, input.VideoID)
		if repoErr != nil {
			return repoErr
		}

		occurredAt = time.Now().UTC()
		eventID = uuid.New()
		event, buildErr := events.NewVideoDeletedEvent(video, eventID, occurredAt, input.Reason)
		if buildErr != nil {
			return fmt.Errorf("build video deleted event: %w", buildErr)
		}

		if err := uc.enqueueOutbox(txCtx, sess, event, eventID, video.VideoID, occurredAt); err != nil {
			return err
		}

		deleted = video
		deleteEvent = event
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("delete video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "delete timeout")
		}
		uc.log.WithContext(ctx).Errorf("delete video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to delete video").WithCause(fmt.Errorf("delete video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("DeleteVideo: video_id=%s", deleted.VideoID)
	return vo.NewVideoDeleted(deleted.VideoID, occurredAt, eventID, deleteEvent.GetVersion(), occurredAt), nil
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

func parseVideoStatus(raw *string) (*po.VideoStatus, error) {
	if raw == nil {
		return nil, nil
	}
	val := strings.TrimSpace(strings.ToLower(*raw))
	status := po.VideoStatus(val)
	switch status {
	case po.VideoStatusPendingUpload,
		po.VideoStatusProcessing,
		po.VideoStatusReady,
		po.VideoStatusPublished,
		po.VideoStatusFailed,
		po.VideoStatusRejected,
		po.VideoStatusArchived:
		return &status, nil
	default:
		return nil, fmt.Errorf("invalid video status: %s", *raw)
	}
}

func parseStageStatus(raw *string) (*po.StageStatus, error) {
	if raw == nil {
		return nil, nil
	}
	val := strings.TrimSpace(strings.ToLower(*raw))
	status := po.StageStatus(val)
	switch status {
	case po.StagePending,
		po.StageProcessing,
		po.StageReady,
		po.StageFailed:
		return &status, nil
	default:
		return nil, fmt.Errorf("invalid stage status: %s", *raw)
	}
}
