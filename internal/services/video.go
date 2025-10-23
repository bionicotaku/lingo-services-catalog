package services

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "video not found")

// VideoRepo 定义 Video 实体的持久化行为接口。
// 由 Repository 层实现，Service 层通过接口调用以保持解耦。
type VideoRepo interface {
	// Create 创建新视频记录（RegisterUpload 用例）
	Create(ctx context.Context, v *po.Video) (*po.Video, error)

	// Update 更新已有视频记录
	Update(ctx context.Context, v *po.Video) (*po.Video, error)

	// FindByID 根据 video_id 查询
	FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error)

	// ListByUploadUser 查询指定用户上传的所有视频
	ListByUploadUser(ctx context.Context, userID uuid.UUID, limit int) ([]*po.Video, error)

	// ListByStatus 根据状态查询（用于监控队列）
	ListByStatus(ctx context.Context, status po.VideoStatus, limit int) ([]*po.Video, error)
}

// VideoUsecase 封装 Video 相关的业务用例逻辑。
// 基于 Catalog Service ARCHITECTURE.md 设计。
type VideoUsecase struct {
	repo VideoRepo   // 本地数据访问接口
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

// RegisterUpload 注册视频上传（对应 CatalogLifecycleService.RegisterUpload RPC）。
//
// 业务流程：
// 1. 生成 video_id（UUID v4）
// 2. 初始化状态为 pending_upload
// 3. 持久化到数据库
// 4. 返回视图对象
//
// 返回值使用 vo 而非 po，避免暴露内部数据结构。
func (uc *VideoUsecase) RegisterUpload(ctx context.Context, uploadUserID uuid.UUID, title, rawFileRef string, description *string) (*vo.VideoRevision, error) {
	// 构造视频实体（初始状态）
	video := &po.Video{
		VideoID:          uuid.New(),
		UploadUserID:     uploadUserID,
		Title:            title,
		Description:      description,
		RawFileReference: rawFileRef,
		Status:           po.VideoStatusPendingUpload,
		MediaStatus:      po.StagePending,
		AnalysisStatus:   po.StagePending,
	}

	// 持久化
	saved, err := uc.repo.Create(ctx, video)
	if err != nil {
		return nil, errors.InternalServer("CREATE_VIDEO_FAILED", "failed to create video record").WithCause(err)
	}

	uc.log.WithContext(ctx).Infof("RegisterUpload: video_id=%s, user=%s, title=%s", saved.VideoID, saved.UploadUserID, saved.Title)

	return &vo.VideoRevision{
		VideoID:   saved.VideoID,
		Status:    string(saved.Status),
		CreatedAt: saved.CreatedAt,
		UpdatedAt: saved.UpdatedAt,
	}, nil
}

// GetVideoDetail 查询视频详情（对应 CatalogQueryService.GetVideoMetadata RPC）。
//
// 返回与用户无关的客观元数据（媒体、AI 字段等）。
func (uc *VideoUsecase) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	video, err := uc.repo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer("QUERY_VIDEO_FAILED", "failed to query video").WithCause(err)
	}

	uc.log.WithContext(ctx).Infof("GetVideoDetail: video_id=%s, status=%s", video.VideoID, video.Status)

	return &vo.VideoDetail{
		VideoID:          video.VideoID,
		Title:            video.Title,
		Description:      video.Description,
		Status:           string(video.Status),
		MediaStatus:      string(video.MediaStatus),
		AnalysisStatus:   string(video.AnalysisStatus),
		ThumbnailURL:     video.ThumbnailURL,
		DurationMicros:   video.DurationMicros,
		CreatedAt:        video.CreatedAt,
		UpdatedAt:        video.UpdatedAt,
	}, nil
}
