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
//
// 流程：
//  1. 构造仓储输入并开启事务。
//  2. 写数据库主表，生成领域事件（VideoCreated）。
//  3. 调用 enqueueOutbox 将事件写入 Outbox。
//  4. 事务提交后返回视图对象。
func (uc *VideoUsecase) CreateVideo(ctx context.Context, input CreateVideoInput) (*vo.VideoCreated, error) {
	// 1) 准备仓储交互所需的输入结构，保持 Service 与仓储层解耦。
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	// 用于承载事务内生成的数据库对象与事件。
	var created *po.Video
	var createdEvent *events.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		// 2) 持久化主记录；失败时直接返回以触发事务回滚。
		video, repoErr := uc.repo.Create(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		// 3) 计算事件发生时间：优先使用数据库写入的 CreatedAt，保证版本单调。
		occurredAt = video.CreatedAt.UTC()
		if occurredAt.IsZero() {
			// 数据库未回写时间戳时（理论上不应该发生），退回当前时间。
			occurredAt = time.Now().UTC()
		}
		// 为事件生成唯一 ID；后续 Outbox 及 Idempotency 依赖该值。
		eventID = uuid.New()
		event, buildErr := events.NewVideoCreatedEvent(video, eventID, occurredAt)
		if buildErr != nil {
			return fmt.Errorf("build video created event: %w", buildErr)
		}

		// 4) 将事件写入 Outbox，确保与主表写操作在同一事务中提交/回滚。
		if err := uc.enqueueOutbox(txCtx, sess, event, occurredAt); err != nil {
			return err
		}

		// 将事务内结果写入闭包外部变量，供事务结束后使用。
		created = video
		createdEvent = event
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// 写入超时：输出警告日志并映射为 504。
			uc.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}
		// 其它错误：记录详细日志并包装为 500。
		uc.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	// 事务成功后记录结构化日志，并返回视图对象（含事件版本）。
	uc.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s status=%s", created.VideoID, created.Title, created.Status)
	return vo.NewVideoCreated(created, eventID, createdEvent.Version, occurredAt), nil
}

// UpdateVideo 更新视频元数据并写入 Outbox。
//
// 流程：
//  1. 校验输入（至少有一个字段变化、时长非负等）。
//  2. 解析枚举值，与仓储交互更新主表。
//  3. 构造 VideoUpdated 领域事件并写入 Outbox。
//  4. 返回更新后的视图对象。
func (uc *VideoUsecase) UpdateVideo(ctx context.Context, input UpdateVideoInput) (*vo.VideoUpdated, error) {
	// 0) 基础校验：必须至少存在一个待更新字段。
	if !hasUpdateFields(input) {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "no fields to update")
	}

	// 0.1) 时长需为非负。
	if input.DurationMicros != nil && *input.DurationMicros < 0 {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "duration_micros must be non-negative")
	}

	// 0.2) 解析状态枚举，校验合法值。
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

	// 1) 准备仓储更新输入。
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
	var updateEvent *events.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err = uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		// 2) 更新主记录；若不存在则返回 ErrVideoNotFound。
		video, repoErr := uc.repo.Update(txCtx, sess, repoInput)
		if repoErr != nil {
			return repoErr
		}

		// 3) 构建领域事件：事件时间使用 UpdatedAt，fallback 到当前时间确保版本更新。
		occurredAt = video.UpdatedAt.UTC()
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}

		// 构建版本化事件所需的变更集合，保留所有可能被更新的字段。
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

		// 4) 写入 Outbox，确保与数据库变更同事务提交。
		if err := uc.enqueueOutbox(txCtx, sess, event, occurredAt); err != nil {
			return err
		}

		updated = video
		updateEvent = event
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			// 映射为业务层的 NotFound 哨兵错误。
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			// 更新执行超时，输出告警日志。
			uc.log.WithContext(ctx).Warnf("update video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "update timeout")
		}
		// 其它错误统一包装并记录日志。
		uc.log.WithContext(ctx).Errorf("update video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to update video").WithCause(fmt.Errorf("update video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("UpdateVideo: video_id=%s", updated.VideoID)
	return vo.NewVideoUpdated(updated, eventID, updateEvent.Version, occurredAt), nil
}

// DeleteVideo 删除视频并记录事件。
//
// 流程：
//  1. 删除主表记录。
//  2. 生成 VideoDeleted 领域事件并写入 Outbox。
//  3. 返回删除结果视图对象。
func (uc *VideoUsecase) DeleteVideo(ctx context.Context, input DeleteVideoInput) (*vo.VideoDeleted, error) {
	// 删除流程同样需要在事务内完成，准备外部变量承载结果。
	var deleted *po.Video
	var deleteEvent *events.DomainEvent
	var eventID uuid.UUID
	var occurredAt time.Time

	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		// 1) 删除主记录。仓储返回被删除的实体，用于构造领域事件。
		video, repoErr := uc.repo.Delete(txCtx, sess, input.VideoID)
		if repoErr != nil {
			return repoErr
		}

		// 2) 构建领域事件：删除事件使用当前时间，保证版本递增。
		occurredAt = time.Now().UTC()
		eventID = uuid.New()
		event, buildErr := events.NewVideoDeletedEvent(video, eventID, occurredAt, input.Reason)
		if buildErr != nil {
			return fmt.Errorf("build video deleted event: %w", buildErr)
		}

		// 3) 写入 Outbox，连同删除操作一起提交。
		if err := uc.enqueueOutbox(txCtx, sess, event, occurredAt); err != nil {
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
			// 删除操作超时：记录告警，并返回网关超时。
			uc.log.WithContext(ctx).Warnf("delete video timeout: video_id=%s", input.VideoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "delete timeout")
		}
		// 其它错误统一处理。
		uc.log.WithContext(ctx).Errorf("delete video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to delete video").WithCause(fmt.Errorf("delete video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("DeleteVideo: video_id=%s", deleted.VideoID)
	return vo.NewVideoDeleted(deleted.VideoID, occurredAt, eventID, deleteEvent.Version, occurredAt), nil
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
