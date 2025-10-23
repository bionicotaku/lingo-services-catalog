// Package repositories 实现数据访问层，封装 sqlc 生成的查询方法。
package repositories

import (
	"context"
	"errors"

	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VideoRepository 实现 services.VideoRepo 接口。
// 基于 sqlc 生成的 Queries，封装数据库操作。
type VideoRepository struct {
	db      *pgxpool.Pool       // PostgreSQL 连接池
	queries *catalogsql.Queries // sqlc 生成的查询对象
}

// NewVideoRepository 构造 VideoRepository 实例。
// 通过 Wire 注入数据库连接池。
func NewVideoRepository(db *pgxpool.Pool) services.VideoRepo {
	return &VideoRepository{
		db:      db,
		queries: catalogsql.New(db),
	}
}

// FindByID 根据 video_id 查询视频详情。
// 实现 services.VideoRepo 接口。
//
// 错误处理：
//   - pgx.ErrNoRows → services.ErrVideoNotFound
//   - 其他数据库错误原样返回
func (r *VideoRepository) FindByID(ctx context.Context, videoID uuid.UUID) (*catalogsql.CatalogVideo, error) {
	video, err := r.queries.FindVideoByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, services.ErrVideoNotFound
		}
		return nil, err
	}
	return &video, nil
}
