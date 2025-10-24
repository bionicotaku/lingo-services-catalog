package services

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
	"github.com/bionicotaku/kratos-template/internal/repositories"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(videov1.ErrorReason_VIDEO_NOT_FOUND.String(), "video not found")

// VideoRepo 定义 Video 实体的持久化行为接口。
// 由 Repository 层实现，使用 sqlc 生成的查询方法。
type VideoRepo interface {
	FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error)
}

// VideoUsecase 封装 Video 相关的业务用例逻辑。
// 基于 Catalog Service ARCHITECTURE.md 设计。
type VideoUsecase struct {
	repo VideoRepo   // 数据访问接口（基于 sqlc）
	log  *log.Helper // 结构化日志辅助器
}

// NewVideoUsecase 构造一个 Video 业务用例实例。
// 通过 Wire 注入 repo 和 logger，实现依赖倒置。
func NewVideoUsecase(repo VideoRepo, logger log.Logger) *VideoUsecase {
	return &VideoUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// GetVideoDetail 查询视频详情（对应 CatalogQueryService.GetVideoMetadata RPC）。
//
// 返回与用户无关的客观元数据（媒体、AI 字段等）。
// 数据转换由 VO 层负责（NewVideoDetail 构造函数）。
func (uc *VideoUsecase) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	video, err := uc.repo.FindByID(ctx, videoID)
	if err != nil {
		// 检查是否为视频未找到错误
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}

		// 检查是否为超时错误
		if errors.Is(err, context.DeadlineExceeded) {
			uc.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_QUERY_TIMEOUT.String(), "query timeout")
		}

		// 其他内部错误
		uc.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	uc.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", video.VideoID, video.Status)

	return vo.NewVideoDetail(video), nil
}
