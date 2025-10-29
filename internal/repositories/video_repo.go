// Package repositories 实现数据访问层，封装 sqlc 生成的查询方法。
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	VisibilityStatus *string
	PublishAt        *time.Time
}

// UpdateVideoInput 表示可选更新字段的集合。
type UpdateVideoInput struct {
	VideoID           uuid.UUID
	Title             *string
	Description       *string
	Status            *po.VideoStatus
	MediaStatus       *po.StageStatus
	AnalysisStatus    *po.StageStatus
	DurationMicros    *int64
	EncodedResolution *string
	EncodedBitrate    *int32
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	Tags              []string
	RawSubtitleURL    *string
	ErrorMessage      *string
	MediaJobID        *string
	MediaEmittedAt    *time.Time
	AnalysisJobID     *string
	AnalysisEmittedAt *time.Time
	RawFileSize       *int64
	RawResolution     *string
	RawBitrate        *int32
	VisibilityStatus  *string
	PublishAt         *time.Time
}

// ListPublicVideosInput 描述公开视频分页查询参数。
type ListPublicVideosInput struct {
	Limit           int32
	CursorCreatedAt *time.Time
	CursorVideoID   *uuid.UUID
}

// ListUserUploadsInput 描述用户上传分页查询参数。
type ListUserUploadsInput struct {
	UploadUserID    uuid.UUID
	Limit           int32
	CursorCreatedAt *time.Time
	CursorVideoID   *uuid.UUID
	StatusFilter    []po.VideoStatus
	StageFilter     []po.StageStatus
}

// Create 创建新视频记录，video_id 由数据库自动生成。
func (r *VideoRepository) Create(ctx context.Context, sess txmanager.Session, input CreateVideoInput) (*po.Video, error) {
	params := mappers.BuildCreateVideoParams(
		input.UploadUserID,
		input.Title,
		input.RawFileReference,
		input.Description,
		input.VisibilityStatus,
		input.PublishAt,
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

// Update 根据输入字段对视频进行部分更新，返回更新后的实体。
func (r *VideoRepository) Update(ctx context.Context, sess txmanager.Session, input UpdateVideoInput) (*po.Video, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.UpdateVideo(ctx, mappers.BuildUpdateVideoParams(
		input.VideoID,
		input.Title,
		input.Description,
		input.ThumbnailURL,
		input.HLSMasterPlaylist,
		input.Difficulty,
		input.Summary,
		input.RawSubtitleURL,
		input.ErrorMessage,
		input.Status,
		input.MediaStatus,
		input.AnalysisStatus,
		input.RawFileSize,
		input.RawResolution,
		input.RawBitrate,
		input.DurationMicros,
		input.EncodedResolution,
		input.EncodedBitrate,
		input.MediaJobID,
		input.AnalysisJobID,
		input.MediaEmittedAt,
		input.AnalysisEmittedAt,
		input.Tags,
		input.VisibilityStatus,
		input.PublishAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("update video failed: video_id=%s err=%v", input.VideoID, err)
		return nil, fmt.Errorf("update video: %w", err)
	}

	r.log.WithContext(ctx).Infof("video updated: video_id=%s", record.VideoID)
	return mappers.VideoFromCatalog(record), nil
}

// Delete 删除视频记录并返回被删除的实体快照。
func (r *VideoRepository) Delete(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.DeleteVideo(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("delete video failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("delete video: %w", err)
	}

	r.log.WithContext(ctx).Infof("video deleted: video_id=%s", record.VideoID)
	return mappers.VideoFromCatalog(record), nil
}

// GetLifecycleSnapshot 返回生命周期服务需要的完整视频快照（不做状态过滤）。
func (r *VideoRepository) GetLifecycleSnapshot(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.Video, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetVideoLifecycleSnapshot(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("get lifecycle snapshot failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("get lifecycle snapshot: %w", err)
	}
	return mappers.VideoFromCatalog(record), nil
}

// GetMetadata 返回视频的媒体/AI 元数据。
func (r *VideoRepository) GetMetadata(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoMetadata, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	row, err := queries.GetVideoMetadata(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("get video metadata failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("get video metadata: %w", err)
	}
	return mappers.VideoMetadataFromRow(row), nil
}

// ListPublicVideos 返回公开可见的视频列表。
func (r *VideoRepository) ListPublicVideos(ctx context.Context, sess txmanager.Session, input ListPublicVideosInput) ([]po.VideoListEntry, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var cursorCreated interface{}
	if input.CursorCreatedAt != nil {
		cursorCreated = input.CursorCreatedAt.UTC()
	}
	params := catalogsql.ListPublicVideosParams{
		CursorCreatedAt: cursorCreated,
		CursorVideoID:   toPgUUID(input.CursorVideoID),
		Limit:           limit,
	}

	rows, err := queries.ListPublicVideos(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list public videos: %w", err)
	}

	items := make([]po.VideoListEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, po.VideoListEntry{
			VideoID:          row.VideoID,
			Title:            row.Title,
			Status:           row.Status,
			MediaStatus:      row.MediaStatus,
			AnalysisStatus:   row.AnalysisStatus,
			CreatedAt:        row.CreatedAt.Time,
			UpdatedAt:        row.UpdatedAt.Time,
			VisibilityStatus: row.VisibilityStatus,
			PublishAt:        timestamptzPtr(row.PublishAt),
		})
	}
	return items, nil
}

// ListUserUploads 返回指定用户的视频。
func (r *VideoRepository) ListUserUploads(ctx context.Context, sess txmanager.Session, input ListUserUploadsInput) ([]po.MyUploadEntry, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var cursorCreated interface{}
	if input.CursorCreatedAt != nil {
		cursorCreated = input.CursorCreatedAt.UTC()
	}

	params := catalogsql.ListUserUploadsParams{
		UploadUserID:    input.UploadUserID,
		StatusFilter:    toStatusFilter(input.StatusFilter),
		StageFilter:     toStageFilter(input.StageFilter),
		CursorCreatedAt: cursorCreated,
		CursorVideoID:   toPgUUID(input.CursorVideoID),
		Limit:           limit,
	}

	rows, err := queries.ListUserUploads(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list user uploads: %w", err)
	}

	items := make([]po.MyUploadEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, po.MyUploadEntry{
			VideoID:          row.VideoID,
			Title:            row.Title,
			Status:           row.Status,
			MediaStatus:      row.MediaStatus,
			AnalysisStatus:   row.AnalysisStatus,
			Version:          row.Version,
			CreatedAt:        row.CreatedAt.Time,
			UpdatedAt:        row.UpdatedAt.Time,
			VisibilityStatus: row.VisibilityStatus,
			PublishAt:        timestamptzPtr(row.PublishAt),
		})
	}
	return items, nil
}

// FindPublishedByID 返回仅对外可见的视频概要（限制在 ready/published）。
func (r *VideoRepository) FindPublishedByID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoReadyView, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.FindPublishedVideo(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoNotFound
		}
		r.log.WithContext(ctx).Errorf("find published video failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("find published video: %w", err)
	}
	return mappers.VideoReadyViewFromFindRow(record), nil
}

func toPgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	var b [16]byte
	copy(b[:], id[:])
	return pgtype.UUID{Bytes: b, Valid: true}
}

func timestamptzPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	value := ts.Time
	return &value
}

func toStatusFilter(filter []po.VideoStatus) interface{} {
	if len(filter) == 0 {
		return nil
	}
	result := make([]string, len(filter))
	for i, s := range filter {
		result[i] = string(s)
	}
	return result
}

func toStageFilter(filter []po.StageStatus) interface{} {
	if len(filter) == 0 {
		return nil
	}
	result := make([]string, len(filter))
	for i, s := range filter {
		result[i] = string(s)
	}
	return result
}
