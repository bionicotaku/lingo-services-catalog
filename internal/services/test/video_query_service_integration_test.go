package services_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestVideoQueryService_GetVideoDetail_WithStats(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyAllMigrations(ctx, t, pool)
	ensureAuthSchema(ctx, t, pool)

	logger := log.NewStdLogger(io.Discard)
	videoRepo := repositories.NewVideoRepository(pool, logger)
	userStateRepo := repositories.NewVideoUserStatesRepository(pool, logger)
	statsRepo := repositories.NewVideoEngagementStatsRepository(pool, logger)

	txCfg := configloader.ProvideTxConfig(configloader.RuntimeConfig{
		Database: configloader.DatabaseConfig{
			Transaction: configloader.TransactionConfig{},
		},
	})
	txMgrComponent, cleanupTx, err := txmanager.NewComponent(txCfg, pool, logger)
	require.NoError(t, err)
	t.Cleanup(cleanupTx)
	txMgr := txmanager.ProvideManager(txMgrComponent)

	service := services.NewVideoQueryService(videoRepo, userStateRepo, statsRepo, txMgr, logger)

	videoID := uuid.New()
	uploadUserID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	_, err = pool.Exec(ctx, `
        INSERT INTO catalog.videos (
            video_id, upload_user_id, title, raw_file_reference,
            status, media_status, analysis_status,
            created_at, updated_at, version
        ) VALUES ($1, $2, $3, 'gs://bucket/test.mp4',
                  'published', 'ready', 'ready',
                  $4, $4, 1)
    `, videoID, uploadUserID, "Stats Integration Video", now)
	require.NoError(t, err)

	userID := uuid.New()
	_, err = pool.Exec(ctx, `
        INSERT INTO catalog.video_user_engagements_projection (
            user_id, video_id, has_liked, has_bookmarked, liked_occurred_at, bookmarked_occurred_at, updated_at
        ) VALUES ($1, $2, true, true, $3, $3, $3)
    `, userID, videoID, now)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
        INSERT INTO catalog.video_engagement_stats_projection (
            video_id, like_count, bookmark_count, watch_count, unique_watchers,
            first_watch_at, last_watch_at, updated_at
        ) VALUES ($1, 5, 2, 7, 3, $2, $2, $2)
        ON CONFLICT (video_id)
        DO UPDATE SET like_count = EXCLUDED.like_count
    `, videoID, now)
	require.NoError(t, err)

	ctxWithMeta := metadata.Inject(ctx, metadata.HandlerMetadata{UserID: userID.String()})
	detail, detailMeta, err := service.GetVideoDetail(ctxWithMeta, videoID)
	require.NoError(t, err)
	require.Equal(t, videoID, detail.VideoID)
	require.True(t, detail.HasLiked)
	require.True(t, detail.HasBookmarked)
	require.EqualValues(t, 5, detail.LikeCount)
	require.EqualValues(t, 2, detail.BookmarkCount)
	require.EqualValues(t, 7, detail.WatchCount)
	require.EqualValues(t, 3, detail.UniqueWatchers)
	require.NotNil(t, detailMeta)
	require.EqualValues(t, 5, detailMeta.LikeCount)
	require.EqualValues(t, 2, detailMeta.BookmarkCount)
	require.EqualValues(t, 7, detailMeta.WatchCount)

	metaOnly, err := service.GetVideoMetadata(ctx, videoID)
	require.NoError(t, err)
	require.EqualValues(t, 5, metaOnly.LikeCount)
	require.EqualValues(t, 2, metaOnly.BookmarkCount)
	require.EqualValues(t, 7, metaOnly.WatchCount)
}

func startPostgres(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_DB":       "catalog",
		},
		WaitingFor: wait.ForSQL("5432/tcp", "pgx", func(host string, port nat.Port) string {
			return fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skip integration: cannot start postgres container: %v", err)
		return "", func() {}
	}

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
	cleanup := func() {
		termCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = container.Terminate(termCtx)
	}
	return dsn, cleanup
}

func applyAllMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	var paths []string
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		paths = append(paths, filepath.Join(migrationsDir, f.Name()))
	}
	sort.Strings(paths)

	for _, path := range paths {
		sqlBytes, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		_, execErr := pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, execErr, "apply migration %s", filepath.Base(path))
	}
}

func ensureAuthSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `create schema if not exists auth`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
        create table if not exists auth.users (
            id uuid primary key,
            email text
        )
    `)
	require.NoError(t, err)
}
