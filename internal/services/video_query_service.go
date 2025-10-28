package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// VideoQueryRepo 定义读模型所需的访问接口。
type VideoQueryRepo interface {
	FindPublishedByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error)
	GetMetadata(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoMetadata, error)
	ListPublicVideos(ctx context.Context, sess txmanager.Session, input repositories.ListPublicVideosInput) ([]po.VideoListEntry, error)
	ListUserUploads(ctx context.Context, sess txmanager.Session, input repositories.ListUserUploadsInput) ([]po.MyUploadEntry, error)
}

// VideoQueryService 封装视频只读用例。
type VideoQueryService struct {
	repo      VideoQueryRepo
	userState *repositories.VideoUserStatesRepository
	txManager txmanager.Manager
	log       *log.Helper
}

// NewVideoQueryService 构造视频查询服务。
func NewVideoQueryService(repo VideoQueryRepo, userState *repositories.VideoUserStatesRepository, tx txmanager.Manager, logger log.Logger) *VideoQueryService {
	return &VideoQueryService{
		repo:      repo,
		userState: userState,
		txManager: tx,
		log:       log.NewHelper(logger),
	}
}

// GetVideoMetadata 查询视频的媒体/AI 元数据。
func (s *VideoQueryService) GetVideoMetadata(ctx context.Context, videoID uuid.UUID) (*vo.VideoMetadata, error) {
	var record *po.VideoMetadata
	err := s.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		record, repoErr = s.repo.GetMetadata(txCtx, sess, videoID)
		return repoErr
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("get video metadata timeout: video_id=%s", videoID)
			return nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		s.log.WithContext(ctx).Errorf("get video metadata failed: video_id=%s err=%v", videoID, err)
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video metadata").WithCause(fmt.Errorf("get video metadata: %w", err))
	}
	return vo.NewVideoMetadataFromPO(record), nil
}

// GetVideoDetail 查询视频详情（优先使用投影表）。
func (s *VideoQueryService) GetVideoDetail(ctx context.Context, videoID uuid.UUID) (*vo.VideoDetail, *vo.VideoMetadata, error) {
	var (
		videoView   *po.VideoReadyView
		state       *po.VideoUserState
		metadataRow *po.VideoMetadata
	)
	var userID *uuid.UUID
	if meta, ok := metadata.FromContext(ctx); ok {
		if meta.InvalidUserInfo {
			return nil, nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "invalid user info metadata")
		}
		if parsed, ok := meta.UserUUID(); ok {
			userID = &parsed
		} else if strings.TrimSpace(meta.UserID) != "" {
			return nil, nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "invalid user id metadata")
		}
	}
	err := s.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		videoView, repoErr = s.repo.FindPublishedByID(txCtx, sess, videoID)
		if repoErr != nil {
			return repoErr
		}
		metadataRow, repoErr = s.repo.GetMetadata(txCtx, sess, videoID)
		if repoErr != nil {
			return repoErr
		}
		if userID != nil && s.userState != nil {
			var err error
			state, err = s.userState.Get(txCtx, sess, *userID, videoID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, nil, ErrVideoNotFound
		}
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("get video detail timeout: video_id=%s", videoID)
			return nil, nil, errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		s.log.WithContext(ctx).Errorf("get video detail failed: video_id=%s err=%v", videoID, err)
		return nil, nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), "failed to query video").WithCause(fmt.Errorf("find video by id: %w", err))
	}

	s.log.WithContext(ctx).Debugf("GetVideoDetail: video_id=%s, status=%s", videoView.VideoID, videoView.Status)
	detail := vo.NewVideoDetail(videoView)
	if detail == nil {
		return nil, nil, ErrVideoNotFound
	}
	if state != nil {
		detail.HasLiked = state.HasLiked
		detail.HasBookmarked = state.HasBookmarked
	}
	return detail, vo.NewVideoMetadataFromPO(metadataRow), nil
}

// ListUserPublicVideos 返回公开视频列表。
func (s *VideoQueryService) ListUserPublicVideos(ctx context.Context, pageSize int32, pageToken string) ([]vo.VideoListItem, string, error) {
	limit := clampPageSize(pageSize)
	cursor, err := decodeCursor(pageToken)
	if err != nil {
		return nil, "", errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "invalid page_token")
	}

	input := repositories.ListPublicVideosInput{
		Limit: limit + 1,
	}
	if cursor != nil {
		input.CursorCreatedAt = &cursor.CreatedAt
		input.CursorVideoID = &cursor.VideoID
	}

	var items []po.VideoListEntry
	err = s.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		items, repoErr = s.repo.ListPublicVideos(txCtx, sess, input)
		return repoErr
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("list public videos timeout")
			return nil, "", errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		return nil, "", errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("list public videos: %v", err))
	}

	var nextToken string
	if len(items) > int(limit) {
		last := items[limit]
		nextToken = encodeCursor(last.CreatedAt, last.VideoID)
		items = items[:limit]
	}

	voItems := make([]vo.VideoListItem, 0, len(items))
	for _, it := range items {
		voItems = append(voItems, vo.VideoListItem{
			VideoID:        it.VideoID,
			Title:          it.Title,
			Status:         string(it.Status),
			MediaStatus:    string(it.MediaStatus),
			AnalysisStatus: string(it.AnalysisStatus),
			CreatedAt:      it.CreatedAt,
			UpdatedAt:      it.UpdatedAt,
		})
	}
	return voItems, nextToken, nil
}

// ListMyUploads 返回用户上传列表。
func (s *VideoQueryService) ListMyUploads(ctx context.Context, pageSize int32, pageToken string, statusFilter []po.VideoStatus, stageFilter []po.StageStatus) ([]vo.MyUploadListItem, string, error) {
	meta, _ := metadata.FromContext(ctx)
	if meta.InvalidUserInfo {
		return nil, "", errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "invalid user info metadata")
	}
	rawUserID := strings.TrimSpace(meta.UserID)
	if rawUserID == "" {
		return nil, "", errors.Unauthorized(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "user_id required")
	}
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		return nil, "", errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "invalid user id")
	}
	limit := clampPageSize(pageSize)
	cursor, err := decodeCursor(pageToken)
	if err != nil {
		return nil, "", errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "invalid page_token")
	}

	input := repositories.ListUserUploadsInput{
		UploadUserID: userID,
		Limit:        limit + 1,
		StatusFilter: statusFilter,
		StageFilter:  stageFilter,
	}
	if cursor != nil {
		input.CursorCreatedAt = &cursor.CreatedAt
		input.CursorVideoID = &cursor.VideoID
	}

	var rows []po.MyUploadEntry
	err = s.txManager.WithinReadOnlyTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		var repoErr error
		rows, repoErr = s.repo.ListUserUploads(txCtx, sess, input)
		return repoErr
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.WithContext(ctx).Warnf("list my uploads timeout")
			return nil, "", errors.GatewayTimeout(videov1.ErrorReason_ERROR_REASON_QUERY_TIMEOUT.String(), "query timeout")
		}
		return nil, "", errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("list my uploads: %v", err))
	}

	var nextToken string
	if len(rows) > int(limit) {
		last := rows[limit]
		nextToken = encodeCursor(last.CreatedAt, last.VideoID)
		rows = rows[:limit]
	}

	voItems := make([]vo.MyUploadListItem, 0, len(rows))
	for _, row := range rows {
		voItems = append(voItems, vo.MyUploadListItem{
			VideoID:        row.VideoID,
			Title:          row.Title,
			Status:         string(row.Status),
			MediaStatus:    string(row.MediaStatus),
			AnalysisStatus: string(row.AnalysisStatus),
			Version:        row.Version,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
	}
	return voItems, nextToken, nil
}

func clampPageSize(size int32) int32 {
	if size <= 0 {
		return 20
	}
	if size > 100 {
		return 100
	}
	return size
}

type pageCursor struct {
	CreatedAt time.Time `json:"created_at"`
	VideoID   uuid.UUID `json:"video_id"`
}

func encodeCursor(created time.Time, id uuid.UUID) string {
	payload, _ := json.Marshal(pageCursor{CreatedAt: created.UTC(), VideoID: id})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(token string) (*pageCursor, error) {
	if token == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var cursor pageCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}
