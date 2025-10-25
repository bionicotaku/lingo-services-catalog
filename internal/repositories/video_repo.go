// Package repositories 实现数据访问层，封装 sqlc 生成的查询方法。
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrVideoNotFound 表示请求的视频不存在。
var ErrVideoNotFound = errors.New("video not found")

// VideoRepository 提供视频相关的持久化访问能力。
type VideoRepository struct {
	db      *pgxpool.Pool
	queries *catalogsql.Queries
	log     *log.Helper
}

// NewVideoRepository 构造 VideoRepository 实例（供 Wire 注入使用）。
func NewVideoRepository(db *pgxpool.Pool, logger log.Logger) *VideoRepository {
	return &VideoRepository{
		db:      db,
		queries: catalogsql.New(db),
		log:     log.NewHelper(logger),
	}
}

// CreateVideoInput 表示创建视频的输入参数。
type CreateVideoInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
}

// Create 创建新视频记录，video_id 由数据库自动生成。
func (r *VideoRepository) Create(ctx context.Context, sess txmanager.Session, input CreateVideoInput) (*po.Video, error) {
	params := mappers.BuildCreateVideoParams(
		input.UploadUserID,
		input.Title,
		input.RawFileReference,
		input.Description,
	)

	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.CreateVideo(ctx, params)
	if err != nil {
		r.log.WithContext(ctx).Errorf("create video failed: title=%s err=%v", input.Title, err)
		return nil, fmt.Errorf("create video: %w", err)
	}

	r.log.WithContext(ctx).Infof("video created: video_id=%s title=%s", record.VideoID, record.Title)
	return mappers.VideoFromCatalog(record), nil
}

// FindByID 根据 video_id 从只读视图查询视频详情（仅返回 ready/published 状态的视频）。
func (r *VideoRepository) FindByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.FindVideoByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("find video by id failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("find video by id: %w", err)
	}
	return mappers.VideoReadyViewFromCatalog(record), nil
}
