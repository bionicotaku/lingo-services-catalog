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

// VideoEngagementStatsRepository 维护 catalog.video_engagement_stats_projection 投影。
type VideoEngagementStatsRepository struct {
	db      *pgxpool.Pool
	queries *catalogsql.Queries
	log     *log.Helper
}

// NewVideoEngagementStatsRepository 构造仓储。
func NewVideoEngagementStatsRepository(db *pgxpool.Pool, logger log.Logger) *VideoEngagementStatsRepository {
	return &VideoEngagementStatsRepository{
		db:      db,
		queries: catalogsql.New(db),
		log:     log.NewHelper(logger),
	}
}

// StatsDelta 表示需要应用的增量。
type StatsDelta struct {
	LikeDelta          int64
	BookmarkDelta      int64
	WatchDelta         int64
	UniqueWatcherDelta int64
	FirstWatchAt       *time.Time
	LastWatchAt        *time.Time
}

// Increment 应用计数增量，返回最新投影。
func (r *VideoEngagementStatsRepository) Increment(ctx context.Context, sess txmanager.Session, videoID uuid.UUID, delta StatsDelta) (*po.VideoEngagementStatsProjection, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	result, err := queries.IncrementVideoEngagementStats(ctx, catalogsql.IncrementVideoEngagementStatsParams{
		VideoID:            videoID,
		LikeDelta:          delta.LikeDelta,
		BookmarkDelta:      delta.BookmarkDelta,
		WatchDelta:         delta.WatchDelta,
		UniqueWatcherDelta: delta.UniqueWatcherDelta,
		FirstWatchAt:       toPgTimestamptz(delta.FirstWatchAt),
		LastWatchAt:        toPgTimestamptz(delta.LastWatchAt),
	})
	if err != nil {
		return nil, fmt.Errorf("increment video engagement stats: %w", err)
	}
	return mappers.VideoEngagementStatsFromRow(result), nil
}

// MarkWatcher 记录唯一观看者，返回是否首次出现。
func (r *VideoEngagementStatsRepository) MarkWatcher(ctx context.Context, sess txmanager.Session, videoID, userID uuid.UUID, watchTime time.Time) (*po.VideoWatcherRecord, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	row, err := queries.UpsertVideoWatcher(ctx, catalogsql.UpsertVideoWatcherParams{
		VideoID:   videoID,
		UserID:    userID,
		WatchTime: toPgTimestamptz(&watchTime),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert video watcher: %w", err)
	}
	return mappers.VideoWatcherRecordFromRow(row), nil
}

// Get 返回指定视频的当前统计。
func (r *VideoEngagementStatsRepository) Get(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.VideoEngagementStatsProjection, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	row, err := queries.GetVideoEngagementStats(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &po.VideoEngagementStatsProjection{VideoID: videoID}, nil
		}
		return nil, fmt.Errorf("get video engagement stats: %w", err)
	}
	return mappers.VideoEngagementStatsFromRow(row), nil
}

func toPgTimestamptz(ts *time.Time) pgtype.Timestamptz {
	if ts == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{
		Time:  ts.UTC(),
		Valid: true,
	}
}

var _ interface {
	Increment(context.Context, txmanager.Session, uuid.UUID, StatsDelta) (*po.VideoEngagementStatsProjection, error)
	MarkWatcher(context.Context, txmanager.Session, uuid.UUID, uuid.UUID, time.Time) (*po.VideoWatcherRecord, error)
	Get(context.Context, txmanager.Session, uuid.UUID) (*po.VideoEngagementStatsProjection, error)
} = (*VideoEngagementStatsRepository)(nil)
