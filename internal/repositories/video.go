// Package repositories 提供数据访问层实现，负责与持久化存储交互。
// 该层实现 Service 层定义的 Repository 接口，隔离底层存储细节。
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// videoRepo 是 services.VideoRepo 接口的实现。
// 使用 pgxpool.Pool 进行数据库访问（Supabase PostgreSQL）。
type videoRepo struct {
	pool *pgxpool.Pool // PostgreSQL 连接池
	log  *log.Helper   // 结构化日志辅助器
}

// NewVideoRepo 构造 VideoRepo 接口的实现实例。
// 通过 Wire 注入数据库连接池和 logger。
func NewVideoRepo(pool *pgxpool.Pool, logger log.Logger) services.VideoRepo {
	return &videoRepo{
		pool: pool,
		log:  log.NewHelper(logger),
	}
}

// Create 创建新视频记录。
// 使用 INSERT ... RETURNING 获取数据库生成的时间戳。
func (r *videoRepo) Create(ctx context.Context, v *po.Video) (*po.Video, error) {
	query := `
		INSERT INTO catalog.videos (
			video_id, upload_user_id, title, description, raw_file_reference,
			status, media_status, analysis_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		v.VideoID,
		v.UploadUserID,
		v.Title,
		v.Description,
		v.RawFileReference,
		v.Status,
		v.MediaStatus,
		v.AnalysisStatus,
	).Scan(&v.CreatedAt, &v.UpdatedAt)

	if err != nil {
		r.log.WithContext(ctx).Errorf("Create video failed: %v", err)
		return nil, fmt.Errorf("insert video: %w", err)
	}

	r.log.WithContext(ctx).Infof("Created video: video_id=%s", v.VideoID)
	return v, nil
}

// Update 更新已有视频记录。
// 仅更新基础字段，媒体/AI 字段由专门的更新方法处理。
func (r *videoRepo) Update(ctx context.Context, v *po.Video) (*po.Video, error) {
	query := `
		UPDATE catalog.videos
		SET
			title = $2,
			description = $3,
			status = $4,
			media_status = $5,
			analysis_status = $6,
			error_message = $7
		WHERE video_id = $1
		RETURNING updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		v.VideoID,
		v.Title,
		v.Description,
		v.Status,
		v.MediaStatus,
		v.AnalysisStatus,
		v.ErrorMessage,
	).Scan(&v.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, services.ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("Update video failed: %v", err)
		return nil, fmt.Errorf("update video: %w", err)
	}

	r.log.WithContext(ctx).Infof("Updated video: video_id=%s", v.VideoID)
	return v, nil
}

// FindByID 根据 video_id 查询视频记录。
// 查询不到时返回 ErrVideoNotFound。
func (r *videoRepo) FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error) {
	query := `
		SELECT
			video_id, upload_user_id, created_at, updated_at,
			title, description, raw_file_reference,
			status, media_status, analysis_status,
			raw_file_size, raw_resolution, raw_bitrate,
			duration_micros, encoded_resolution, encoded_bitrate,
			thumbnail_url, hls_master_playlist,
			difficulty, summary, tags, raw_subtitle_url,
			error_message
		FROM catalog.videos
		WHERE video_id = $1
	`

	var v po.Video
	err := r.pool.QueryRow(ctx, query, videoID).Scan(
		&v.VideoID, &v.UploadUserID, &v.CreatedAt, &v.UpdatedAt,
		&v.Title, &v.Description, &v.RawFileReference,
		&v.Status, &v.MediaStatus, &v.AnalysisStatus,
		&v.RawFileSize, &v.RawResolution, &v.RawBitrate,
		&v.DurationMicros, &v.EncodedResolution, &v.EncodedBitrate,
		&v.ThumbnailURL, &v.HLSMasterPlaylist,
		&v.Difficulty, &v.Summary, &v.Tags, &v.RawSubtitleURL,
		&v.ErrorMessage,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, services.ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("FindByID failed: %v", err)
		return nil, fmt.Errorf("query video by id: %w", err)
	}

	return &v, nil
}

// ListByUploadUser 查询指定用户上传的所有视频。
// 按创建时间倒序排列，支持分页限制。
func (r *videoRepo) ListByUploadUser(ctx context.Context, userID uuid.UUID, limit int) ([]*po.Video, error) {
	if limit <= 0 {
		limit = 100 // 默认限制
	}

	query := `
		SELECT
			video_id, upload_user_id, created_at, updated_at,
			title, description, raw_file_reference,
			status, media_status, analysis_status,
			thumbnail_url, duration_micros
		FROM catalog.videos
		WHERE upload_user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		r.log.WithContext(ctx).Errorf("ListByUploadUser failed: %v", err)
		return nil, fmt.Errorf("query videos by user: %w", err)
	}
	defer rows.Close()

	var videos []*po.Video
	for rows.Next() {
		var v po.Video
		err := rows.Scan(
			&v.VideoID, &v.UploadUserID, &v.CreatedAt, &v.UpdatedAt,
			&v.Title, &v.Description, &v.RawFileReference,
			&v.Status, &v.MediaStatus, &v.AnalysisStatus,
			&v.ThumbnailURL, &v.DurationMicros,
		)
		if err != nil {
			r.log.WithContext(ctx).Errorf("Scan video row failed: %v", err)
			return nil, fmt.Errorf("scan video row: %w", err)
		}
		videos = append(videos, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate video rows: %w", err)
	}

	return videos, nil
}

// ListByStatus 根据状态查询视频列表（用于监控队列）。
// 按创建时间正序排列（先进先出）。
func (r *videoRepo) ListByStatus(ctx context.Context, status po.VideoStatus, limit int) ([]*po.Video, error) {
	if limit <= 0 {
		limit = 100 // 默认限制
	}

	query := `
		SELECT
			video_id, upload_user_id, created_at, updated_at,
			title, status, media_status, analysis_status,
			error_message
		FROM catalog.videos
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, status, limit)
	if err != nil {
		r.log.WithContext(ctx).Errorf("ListByStatus failed: %v", err)
		return nil, fmt.Errorf("query videos by status: %w", err)
	}
	defer rows.Close()

	var videos []*po.Video
	for rows.Next() {
		var v po.Video
		err := rows.Scan(
			&v.VideoID, &v.UploadUserID, &v.CreatedAt, &v.UpdatedAt,
			&v.Title, &v.Status, &v.MediaStatus, &v.AnalysisStatus,
			&v.ErrorMessage,
		)
		if err != nil {
			r.log.WithContext(ctx).Errorf("Scan video row failed: %v", err)
			return nil, fmt.Errorf("scan video row: %w", err)
		}
		videos = append(videos, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate video rows: %w", err)
	}

	return videos, nil
}
