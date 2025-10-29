package repositories_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestVideoEngagementStatsRepositoryIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	logger := log.NewStdLogger(io.Discard)
	repo := repositories.NewVideoEngagementStatsRepository(pool, logger)

	videoID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	// 初始点赞/收藏入库
	stats, err := repo.Increment(ctx, nil, videoID, repositories.StatsDelta{
		LikeDelta:     1,
		BookmarkDelta: 1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.LikeCount)
	require.EqualValues(t, 1, stats.BookmarkCount)
	require.EqualValues(t, 0, stats.WatchCount)
	require.EqualValues(t, 0, stats.UniqueWatchers)

	// 收藏撤销，不应小于 0
	stats, err = repo.Increment(ctx, nil, videoID, repositories.StatsDelta{
		BookmarkDelta: -2, // 超过已有数量，最终仍应为 0
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.LikeCount)
	require.EqualValues(t, 0, stats.BookmarkCount)

	// 第一次观看：记录唯一观看者，写入首/末观看时间
	userA := uuid.New()
	watchTimeA := now.Add(2 * time.Minute)
	record, err := repo.MarkWatcher(ctx, nil, videoID, userA, watchTimeA)
	require.NoError(t, err)
	require.True(t, record.Inserted)

	stats, err = repo.Increment(ctx, nil, videoID, repositories.StatsDelta{
		WatchDelta:         1,
		UniqueWatcherDelta: 1,
		FirstWatchAt:       &watchTimeA,
		LastWatchAt:        &watchTimeA,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.WatchCount)
	require.EqualValues(t, 1, stats.UniqueWatchers)
	require.NotNil(t, stats.FirstWatchAt)
	require.WithinDuration(t, watchTimeA, *stats.FirstWatchAt, time.Second)

	// 同一用户再次观看，只更新 watch_count 与 last_watch_at
	laterWatch := watchTimeA.Add(3 * time.Minute)
	record, err = repo.MarkWatcher(ctx, nil, videoID, userA, laterWatch)
	require.NoError(t, err)
	require.False(t, record.Inserted)

	stats, err = repo.Increment(ctx, nil, videoID, repositories.StatsDelta{
		WatchDelta:  1,
		LastWatchAt: &laterWatch,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.WatchCount)
	require.EqualValues(t, 1, stats.UniqueWatchers)
	require.NotNil(t, stats.LastWatchAt)
	require.WithinDuration(t, laterWatch, *stats.LastWatchAt, time.Second)

	// 新用户观看，唯一观看者 +1
	userB := uuid.New()
	watchTimeB := laterWatch.Add(time.Minute)
	record, err = repo.MarkWatcher(ctx, nil, videoID, userB, watchTimeB)
	require.NoError(t, err)
	require.True(t, record.Inserted)

	stats, err = repo.Increment(ctx, nil, videoID, repositories.StatsDelta{
		WatchDelta:         1,
		UniqueWatcherDelta: 1,
		LastWatchAt:        &watchTimeB,
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, stats.WatchCount)
	require.EqualValues(t, 2, stats.UniqueWatchers)
	require.WithinDuration(t, watchTimeA, *stats.FirstWatchAt, time.Second)
	require.WithinDuration(t, watchTimeB, *stats.LastWatchAt, time.Second)

	// Get 查询应返回最新投影
	fetched, err := repo.Get(ctx, nil, videoID)
	require.NoError(t, err)
	require.EqualValues(t, stats.LikeCount, fetched.LikeCount)
	require.EqualValues(t, stats.BookmarkCount, fetched.BookmarkCount)
	require.EqualValues(t, stats.WatchCount, fetched.WatchCount)
	require.EqualValues(t, stats.UniqueWatchers, fetched.UniqueWatchers)

	// 未存在的视频返回默认结构
	missingID := uuid.New()
	empty, err := repo.Get(ctx, nil, missingID)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Equal(t, missingID, empty.VideoID)
	require.EqualValues(t, 0, empty.LikeCount)
	require.EqualValues(t, 0, empty.BookmarkCount)
	require.EqualValues(t, 0, empty.WatchCount)
	require.EqualValues(t, 0, empty.UniqueWatchers)
}
