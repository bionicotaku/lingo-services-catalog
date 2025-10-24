package services

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
	"github.com/bionicotaku/kratos-template/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(videov1.ErrorReason_ERROR_REASON_VIDEO_NOT_FOUND.String(), "video not found")

// VideoRepo 定义 Video 实体的持久化行为接口。
// 由 Repository 层实现，使用 sqlc 生成的查询方法。
type VideoRepo interface {
	Create(ctx context.Context, sess txmanager.Session, input repositories.CreateVideoInput) (*po.Video, error)
	FindByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error)
}

// VideoUsecase 封装 Video 相关的业务用例逻辑。
// 基于 Catalog Service ARCHITECTURE.md 设计。
type VideoUsecase struct {
	repo      VideoRepo // 数据访问接口（基于 sqlc）
	txManager txmanager.Manager
	log       *log.Helper // 结构化日志辅助器
}

// NewVideoUsecase 构造一个 Video 业务用例实例。
// 通过 Wire 注入 repo 和 logger，实现依赖倒置。
func NewVideoUsecase(repo VideoRepo, tx txmanager.Manager, logger log.Logger) *VideoUsecase {
	return &VideoUsecase{
		repo:      repo,
		txManager: tx,
		log:       log.NewHelper(logger),
	}
}

// CreateVideoInput 表示创建视频的输入参数。
type CreateVideoInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
}

// CreateVideo 创建新视频记录（对应 VideoCommandService.CreateVideo RPC）。
//
// video_id 由数据库自动生成，返回创建后的视频元数据。
// 初始状态：status=pending_upload, media_status=pending, analysis_status=pending。
func (uc *VideoUsecase) CreateVideo(ctx context.Context, input CreateVideoInput) (*vo.VideoCreated, error) {
	// 构造 Repository 输入参数
	repoInput := repositories.CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}

	var created *po.Video
	err := uc.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		created, repoErr = uc.repo.Create(txCtx, sess, repoInput)
		return repoErr
	})
	if err != nil {
		// 检查是否为超时错误
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("create video timeout: title=%s", input.Title)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "create timeout")
		}

		// 其他内部错误
		uc.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to create video").WithCause(fmt.Errorf("create video: %w", err))
	}

	uc.log.WithContext(ctx).Infof("CreateVideo: video_id=%s title=%s status=%s", created.VideoID, created.Title, created.Status)

	return vo.NewVideoCreated(created), nil
}

// GetVideoDetail 查询视频详情（对应 CatalogQueryService.GetVideoMetadata RPC）。
//
// 从只读视图 videos_ready_view 查询，仅返回 ready/published 状态的视频核心信息。
// 数据转换由 VO 层负责（NewVideoDetail 构造函数）。
func (uc *VideoUsecase) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	var videoView *po.VideoReadyView
	err := uc.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		videoView, repoErr = uc.repo.FindByID(txCtx, sess, videoID)
		return repoErr
	})
	if err != nil {
		// 检查是否为视频未找到错误
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}

		// 检查是否为超时错误
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}

		// 其他内部错误
		uc.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	uc.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", videoView.VideoID, videoView.Status)

	return vo.NewVideoDetail(videoView), nil
}
