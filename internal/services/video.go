package services

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "video not found")

// VideoRepo 定义 Video 实体的持久化行为接口。
// 由 Repository 层实现，使用 sqlc 生成的查询方法。
type VideoRepo interface {
	// FindByID 根据 video_id 查询视频详情
	FindByID(ctx context.Context, videoID uuid.UUID) (*catalogsql.CatalogVideo, error)
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
		if errors.Is(err, ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer("QUERY_VIDEO_FAILED", "failed to query video").WithCause(err)
	}

	uc.log.WithContext(ctx).Infof("GetVideoDetail: video_id=%s, status=%s", video.VideoID, video.Status)

	// VO 层负责转换逻辑，Service 层只关注业务流程
	return vo.NewVideoDetail(video), nil
}
