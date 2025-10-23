// Package repositories 实现数据访问层，封装 sqlc 生成的查询方法。
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"

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

// FindByID 根据 video_id 查询视频详情。
func (r *VideoRepository) FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error) {
	record, err := r.queries.FindVideoByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("find video by id failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("find video by id: %w", err)
	}
	return mappers.VideoFromCatalog(record), nil
}
