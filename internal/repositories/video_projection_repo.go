package repositories

import (
	"context"
	"time"

	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bionicotaku/kratos-template/internal/models/po"
)

// VideoProjection 描述读模型所需字段。
type VideoProjection struct {
	VideoID        uuid.UUID
	Title          string
	Status         po.VideoStatus
	MediaStatus    po.StageStatus
	AnalysisStatus po.StageStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
	OccurredAt     time.Time
}

// VideoProjectionRepository 提供投影表的存取能力。
type VideoProjectionRepository struct {
	baseQueries *catalogsql.Queries
	log         *log.Helper
}

// NewVideoProjectionRepository 构造 VideoProjectionRepository。
func NewVideoProjectionRepository(db *pgxpool.Pool, logger log.Logger) *VideoProjectionRepository {
	return &VideoProjectionRepository{
		baseQueries: catalogsql.New(db),
		log:         log.NewHelper(logger),
	}
}

// Upsert 根据版本号插入或更新投影数据。
func (r *VideoProjectionRepository) Upsert(ctx context.Context, sess txmanager.Session, vp VideoProjection) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := catalogsql.UpsertVideoProjectionParams{
		VideoID:        vp.VideoID,
		Title:          vp.Title,
		Status:         vp.Status,
		MediaStatus:    vp.MediaStatus,
		AnalysisStatus: vp.AnalysisStatus,
		CreatedAt:      timestamptzFromTime(vp.CreatedAt),
		UpdatedAt:      timestamptzFromTime(vp.UpdatedAt),
		Version:        vp.Version,
		OccurredAt:     timestamptzFromTime(vp.OccurredAt),
	}

	return queries.UpsertVideoProjection(ctx, params)
}

// Delete 删除指定视频的投影记录（仅当事件版本不小于当前版本）。
func (r *VideoProjectionRepository) Delete(ctx context.Context, sess txmanager.Session, videoID uuid.UUID, version int64) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	return queries.DeleteVideoProjection(ctx, catalogsql.DeleteVideoProjectionParams{
		VideoID: videoID,
		Version: version,
	})
}

// Get 返回指定视频的投影记录。
func (r *VideoProjectionRepository) Get(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*catalogsql.CatalogVideoProjection, error) {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetVideoProjection(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return &record, nil
}
