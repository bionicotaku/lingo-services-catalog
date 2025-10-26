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

// VideoQueryRepo 定义读模型所需的访问接口。
type VideoQueryRepo interface {
	FindByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error)
}

// VideoQueryService 封装视频只读用例。
type VideoQueryService struct {
	repo      VideoQueryRepo
	txManager txmanager.Manager
	log       *log.Helper
}

// NewVideoQueryService 构造视频查询服务。
func NewVideoQueryService(repo VideoQueryRepo, tx txmanager.Manager, logger log.Logger) *VideoQueryService {
	return &VideoQueryService{
		repo:      repo,
		txManager: tx,
		log:       log.NewHelper(logger),
	}
}

// GetVideoDetail 查询视频详情（优先使用投影表）。
func (s *VideoQueryService) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, error) {
	var videoView *po.VideoReadyView
	err := s.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		videoView, repoErr = s.repo.FindByID(txCtx, sess, videoID)
		return repoErr
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		s.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	s.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", videoView.VideoID, videoView.Status)
	return vo.NewVideoDetail(videoView), nil
}
