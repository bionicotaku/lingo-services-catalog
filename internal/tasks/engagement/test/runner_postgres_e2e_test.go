package engagement_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/pstest"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
	profilev1 "github.com/bionicotaku/lingo-services-profile/api/profile/v1"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type engagementRunnerEnv struct {
	ctx       context.Context
	pool      *pgxpool.Pool
	userRepo  *repositories.VideoUserStatesRepository
	statsRepo *repositories.VideoEngagementStatsRepository
	txMgr     txmanager.Manager
	publisher gcpubsub.Publisher
	cancel    context.CancelFunc
	errCh     chan error
	cleanup   func()
	logger    log.Logger
}

func newEngagementRunnerEnv(t *testing.T) *engagementRunnerEnv {
	t.Helper()

	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	ensureAuthSchema(ctx, t, pool)
	applyMigrations(ctx, t, pool)
	_, err = pool.Exec(ctx, `set search_path to catalog, public`)
	require.NoError(t, err)

	logger := log.NewStdLogger(io.Discard)
	repo := repositories.NewVideoUserStatesRepository(pool, logger)
	statsRepo := repositories.NewVideoEngagementStatsRepository(pool, logger)
	inboxRepo := repositories.NewInboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})
	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	server := pstest.NewServer()

	projectID := "test-project"
	topicID := "profile.engagement.events"
	subscriptionID := "catalog.profile-engagement"

	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = server.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)
	_, err = server.GServer.CreateSubscription(ctx, &pubsubpb.Subscription{Name: subscriptionName, Topic: topicName})
	require.NoError(t, err)

	enableMetrics := true
	component, componentCleanup, err := gcpubsub.NewComponent(ctx, gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		SubscriptionID:   subscriptionID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		EmulatorEndpoint: server.Addr,
	}, gcpubsub.Dependencies{Logger: logger})
	require.NoError(t, err)

	publisher := gcpubsub.ProvidePublisher(component)
	subscriber := gcpubsub.ProvideSubscriber(component)

	runner, err := engagement.NewRunner(engagement.RunnerParams{
		Subscriber: subscriber,
		InboxRepo:  inboxRepo,
		UserRepo:   repo,
		StatsRepo:  statsRepo,
		TxManager:  txMgr,
		Logger:     logger,
		Config: outboxcfg.InboxConfig{
			SourceService:  "profile",
			MaxConcurrency: 4,
		},
	})
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(runCtx)
	}()

	cleanup := func() {
		cancel()
		select {
		case runErr := <-errCh:
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				t.Fatalf("runner returned error: %v", runErr)
			}
		case <-time.After(time.Second):
			t.Fatalf("runner did not stop in time")
		}
		componentCleanup()
		_ = server.Close()
		terminate()
		pool.Close()
	}

	return &engagementRunnerEnv{
		ctx:       ctx,
		pool:      pool,
		userRepo:  repo,
		statsRepo: statsRepo,
		txMgr:     txMgr,
		publisher: publisher,
		cancel:    cancel,
		errCh:     errCh,
		cleanup:   cleanup,
		logger:    logger,
	}
}

func (e *engagementRunnerEnv) Shutdown() {
	if e == nil {
		return
	}
	if e.cleanup != nil {
		e.cleanup()
	}
}

func TestEngagementRunner_WithRealRepository(t *testing.T) {
	t.Parallel()

	env := newEngagementRunnerEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	pool := env.pool
	repo := env.userRepo
	statsRepo := env.statsRepo
	txMgr := env.txMgr
	publisher := env.publisher
	logger := env.logger

	userID := uuid.New()
	videoID := uuid.New()

	_, err := pool.Exec(ctx, `insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing`, userID, "catalog-engagement@test.local")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
        insert into catalog.videos (
            video_id,
            upload_user_id,
            title,
            raw_file_reference,
            status,
            media_status,
            analysis_status
        ) values (
            $1, $2, 'Integration Video', 'gs://test/video.mp4', 'ready', 'ready', 'ready'
        )
        on conflict (video_id) do nothing
    `, videoID, userID)
	require.NoError(t, err)

	baseTime := time.Now().UTC().Add(-5 * time.Minute)

	likeEventID, likePayload, err := buildEngagementAdded(userID, videoID, profilev1.FavoriteType_FAVORITE_TYPE_LIKE, baseTime)
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, likeEventID, likePayload, "profile.engagement.added", videoID))

	bookmarkEventID, bookmarkPayload, err := buildEngagementAdded(userID, videoID, profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK, baseTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, bookmarkEventID, bookmarkPayload, "profile.engagement.added", videoID))

	wait := waitForState(ctx, t, repo, pool, userID, videoID, 15*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && st.HasBookmarked &&
			st.LikedOccurredAt != nil && approxEqual(*st.LikedOccurredAt, baseTime) &&
			st.BookmarkedOccurredAt != nil && approxEqual(*st.BookmarkedOccurredAt, baseTime.Add(2*time.Minute))
	})
	require.NotNil(t, wait)

	removeBookmarkEventID, removeBookmarkPayload, err := buildEngagementRemoved(userID, videoID, profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK, baseTime.Add(4*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, removeBookmarkEventID, removeBookmarkPayload, "profile.engagement.removed", videoID))

	wait = waitForState(ctx, t, repo, pool, userID, videoID, 15*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && !st.HasBookmarked &&
			st.LikedOccurredAt != nil && approxEqual(*st.LikedOccurredAt, baseTime) &&
			st.BookmarkedOccurredAt != nil && approxEqual(*st.BookmarkedOccurredAt, baseTime.Add(4*time.Minute))
	})
	require.NotNil(t, wait)

	watchEventID, watchPayload, err := buildWatchProgressed(userID, videoID, baseTime.Add(6*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, watchEventID, watchPayload, "profile.watch.progressed", videoID))

	stats := waitForStats(ctx, t, statsRepo, videoID, 15*time.Second, func(st *po.VideoEngagementStatsProjection) bool {
		return st.LikeCount == 1 && st.BookmarkCount == 0 && st.WatchCount >= 1 && st.UniqueWatchers >= 1
	})
	require.NotNil(t, stats)
	require.True(t, approxEqualTime(stats.LastWatchAt, baseTime.Add(6*time.Minute)))

	videoRepo := repositories.NewVideoRepository(pool, logger)
	querySvc := services.NewVideoQueryService(videoRepo, repo, statsRepo, txMgr, logger)
	queryCtx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: userID.String()})
	detail, _, err := querySvc.GetVideoDetail(queryCtx, videoID)
	require.NoError(t, err)
	require.True(t, detail.HasLiked)
	require.False(t, detail.HasBookmarked)
	require.EqualValues(t, 1, detail.LikeCount)
	require.EqualValues(t, 0, detail.BookmarkCount)
	require.EqualValues(t, 1, detail.WatchCount)
	require.EqualValues(t, 1, detail.UniqueWatchers)

	assertInboxProcessed(ctx, t, pool, likeEventID)
	assertInboxProcessed(ctx, t, pool, bookmarkEventID)
	assertInboxProcessed(ctx, t, pool, removeBookmarkEventID)
	assertInboxProcessed(ctx, t, pool, watchEventID)
}

func TestEngagementRunner_MetadataProjectionReturnsStats(t *testing.T) {
	env := newEngagementRunnerEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	pool := env.pool
	repo := env.userRepo
	statsRepo := env.statsRepo
	txMgr := env.txMgr
	publisher := env.publisher

	userID := uuid.New()
	videoID := uuid.New()

	_, err := pool.Exec(ctx, `insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing`, userID, "catalog-engagement@test.local")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
        insert into catalog.videos (
            video_id,
            upload_user_id,
            title,
            raw_file_reference,
            status,
            media_status,
            analysis_status
        ) values (
            $1, $2, 'Metadata Video', 'gs://test/video.mp4', 'ready', 'ready', 'ready'
        )
        on conflict (video_id) do nothing
    `, videoID, userID)
	require.NoError(t, err)

	baseTime := time.Now().UTC().Add(-10 * time.Minute)

	likeEventID, likePayload, err := buildEngagementAdded(userID, videoID, profilev1.FavoriteType_FAVORITE_TYPE_LIKE, baseTime)
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, likeEventID, likePayload, "profile.engagement.added", videoID))

	bookmarkEventID, bookmarkPayload, err := buildEngagementAdded(userID, videoID, profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK, baseTime.Add(3*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, bookmarkEventID, bookmarkPayload, "profile.engagement.added", videoID))

	watchEventID, watchPayload, err := buildWatchProgressed(userID, videoID, baseTime.Add(5*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, watchEventID, watchPayload, "profile.watch.progressed", videoID))

	stats := waitForStats(ctx, t, statsRepo, videoID, 15*time.Second, func(st *po.VideoEngagementStatsProjection) bool {
		return st.LikeCount == 1 && st.BookmarkCount == 1 && st.WatchCount >= 1
	})
	require.NotNil(t, stats)

	// ensure user state row exists for metadata detail join
	wait := waitForState(ctx, t, repo, pool, userID, videoID, 15*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && st.HasBookmarked
	})
	require.NotNil(t, wait)

	videoRepo := repositories.NewVideoRepository(pool, env.logger)
	querySvc := services.NewVideoQueryService(videoRepo, repo, statsRepo, txMgr, env.logger)

	meta, err := querySvc.GetVideoMetadata(context.Background(), videoID)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.EqualValues(t, 1, meta.LikeCount)
	require.EqualValues(t, 1, meta.BookmarkCount)
	require.GreaterOrEqual(t, meta.WatchCount, int64(1))

	assertInboxProcessed(ctx, t, pool, likeEventID)
	assertInboxProcessed(ctx, t, pool, bookmarkEventID)
	assertInboxProcessed(ctx, t, pool, watchEventID)
}

func TestEngagementRunner_DetailAggregatesMultipleUsers(t *testing.T) {
	env := newEngagementRunnerEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	pool := env.pool
	repo := env.userRepo
	statsRepo := env.statsRepo
	txMgr := env.txMgr
	publisher := env.publisher

	uploaderID := uuid.New()
	user1 := uuid.New()
	user2 := uuid.New()
	videoID := uuid.New()

	_, err := pool.Exec(ctx, `insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing`, uploaderID, "catalog-engagement@test.local")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
        insert into catalog.videos (
            video_id,
            upload_user_id,
            title,
            raw_file_reference,
            status,
            media_status,
            analysis_status
        ) values (
            $1, $2, 'Aggregates Video', 'gs://test/video.mp4', 'ready', 'ready', 'ready'
        )
        on conflict (video_id) do nothing
    `, videoID, uploaderID)
	require.NoError(t, err)

	baseTime := time.Now().UTC().Add(-8 * time.Minute)

	user1LikeID, user1LikePayload, err := buildEngagementAdded(user1, videoID, profilev1.FavoriteType_FAVORITE_TYPE_LIKE, baseTime)
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, user1LikeID, user1LikePayload, "profile.engagement.added", videoID))

	user1WatchID, user1WatchPayload, err := buildWatchProgressed(user1, videoID, baseTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, user1WatchID, user1WatchPayload, "profile.watch.progressed", videoID))

	user2LikeID, user2LikePayload, err := buildEngagementAdded(user2, videoID, profilev1.FavoriteType_FAVORITE_TYPE_LIKE, baseTime.Add(3*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, user2LikeID, user2LikePayload, "profile.engagement.added", videoID))

	user2BookmarkID, user2BookmarkPayload, err := buildEngagementAdded(user2, videoID, profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK, baseTime.Add(4*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, user2BookmarkID, user2BookmarkPayload, "profile.engagement.added", videoID))

	user2WatchID, user2WatchPayload, err := buildWatchProgressed(user2, videoID, baseTime.Add(5*time.Minute))
	require.NoError(t, err)
	require.NoError(t, publishEvent(ctx, publisher, user2WatchID, user2WatchPayload, "profile.watch.progressed", videoID))

	stats := waitForStats(ctx, t, statsRepo, videoID, 20*time.Second, func(st *po.VideoEngagementStatsProjection) bool {
		return st.LikeCount >= 2 && st.BookmarkCount >= 1 && st.WatchCount >= 2 && st.UniqueWatchers >= 2
	})
	require.NotNil(t, stats)

	waitForState(ctx, t, repo, pool, user1, videoID, 15*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked
	})
	waitForState(ctx, t, repo, pool, user2, videoID, 15*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && st.HasBookmarked
	})

	videoRepo := repositories.NewVideoRepository(pool, env.logger)
	querySvc := services.NewVideoQueryService(videoRepo, repo, statsRepo, txMgr, env.logger)
	queryCtx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: user1.String()})
	detail, meta, err := querySvc.GetVideoDetail(queryCtx, videoID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.NotNil(t, meta)

	require.True(t, detail.HasLiked)
	require.False(t, detail.HasBookmarked)
	require.EqualValues(t, 2, detail.LikeCount)
	require.EqualValues(t, 1, detail.BookmarkCount)
	require.GreaterOrEqual(t, detail.WatchCount, int64(2))
	require.GreaterOrEqual(t, detail.UniqueWatchers, int64(2))
	require.EqualValues(t, 2, meta.LikeCount)
	require.EqualValues(t, 1, meta.BookmarkCount)
	require.GreaterOrEqual(t, meta.WatchCount, int64(2))

	assertInboxProcessed(ctx, t, pool, user1LikeID)
	assertInboxProcessed(ctx, t, pool, user1WatchID)
	assertInboxProcessed(ctx, t, pool, user2LikeID)
	assertInboxProcessed(ctx, t, pool, user2BookmarkID)
	assertInboxProcessed(ctx, t, pool, user2WatchID)
}

func buildEngagementAdded(userID, videoID uuid.UUID, fav profilev1.FavoriteType, occurred time.Time) (uuid.UUID, []byte, error) {
	eventID := uuid.New()
	msg := &profilev1.EngagementAddedEvent{
		EventId:      eventID.String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: fav,
	}
	if !occurred.IsZero() {
		msg.OccurredAt = timestamppb.New(occurred.UTC())
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return eventID, data, nil
}

func buildEngagementRemoved(userID, videoID uuid.UUID, fav profilev1.FavoriteType, occurred time.Time) (uuid.UUID, []byte, error) {
	eventID := uuid.New()
	msg := &profilev1.EngagementRemovedEvent{
		EventId:      eventID.String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: fav,
	}
	if !occurred.IsZero() {
		msg.OccurredAt = timestamppb.New(occurred.UTC())
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return eventID, data, nil
}

func buildWatchProgressed(userID, videoID uuid.UUID, occurred time.Time) (uuid.UUID, []byte, error) {
	eventID := uuid.New()
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	msg := &profilev1.WatchProgressedEvent{
		EventId: eventID.String(),
		UserId:  userID.String(),
		VideoId: videoID.String(),
		Progress: &profilev1.WatchProgress{
			PositionSeconds:   0,
			ProgressRatio:     0,
			TotalWatchSeconds: 0,
			FirstWatchedAt:    timestamppb.New(occurred.UTC()),
			LastWatchedAt:     timestamppb.New(occurred.UTC()),
		},
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return eventID, data, nil
}

func publishEvent(ctx context.Context, publisher gcpubsub.Publisher, eventID uuid.UUID, payload []byte, eventType string, videoID uuid.UUID) error {
	attrs := map[string]string{
		"event_id":       eventID.String(),
		"event_type":     eventType,
		"aggregate_type": "video",
		"aggregate_id":   videoID.String(),
		"schema_version": "v1",
	}
	_, err := publisher.Publish(ctx, gcpubsub.Message{Data: payload, Attributes: attrs})
	return err
}

func waitForState(ctx context.Context, t *testing.T, repo *repositories.VideoUserStatesRepository, pool *pgxpool.Pool, userID, videoID uuid.UUID, timeout time.Duration, predicate func(*po.VideoUserState) bool) *po.VideoUserState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := repo.Get(ctx, nil, userID, videoID)
		if err != nil {
			t.Fatalf("get state failed: %v", err)
		}
		if state != nil && predicate(state) {
			return state
		}
		time.Sleep(50 * time.Millisecond)
	}
	logInboxState(ctx, t, pool)
	return nil
}

func waitForStats(ctx context.Context, t *testing.T, repo *repositories.VideoEngagementStatsRepository, videoID uuid.UUID, timeout time.Duration, predicate func(*po.VideoEngagementStatsProjection) bool) *po.VideoEngagementStatsProjection {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		record, err := repo.Get(ctx, nil, videoID)
		if err != nil {
			t.Fatalf("get stats failed: %v", err)
		}
		if record != nil && predicate(record) {
			return record
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func approxEqual(a, b time.Time) bool {
	const tolerance = 50 * time.Millisecond
	if a.IsZero() || b.IsZero() {
		return false
	}
	return a.Sub(b).Abs() <= tolerance
}

func approxEqualTime(actual *time.Time, expected time.Time) bool {
	if actual == nil {
		return false
	}
	return approxEqual(actual.UTC(), expected)
}

func assertInboxProcessed(ctx context.Context, t *testing.T, pool *pgxpool.Pool, eventID uuid.UUID) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		row := pool.QueryRow(ctx, `select processed_at, last_error from catalog.inbox_events where event_id = $1`, eventID)
		var processedAt *time.Time
		var lastError *string
		if err := row.Scan(&processedAt, &lastError); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			t.Fatalf("fetch inbox event %s failed: %v", eventID, err)
		}
		if processedAt == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if lastError != nil && *lastError != "" {
			t.Fatalf("inbox event %s recorded error: %s", eventID, *lastError)
		}
		return
	}
	t.Fatalf("inbox event %s not processed", eventID)
}

func logInboxState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `select event_id, processed_at, last_error from catalog.inbox_events order by received_at desc`)
	if err != nil {
		t.Logf("query inbox_events failed: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var eventID uuid.UUID
		var processedAt *time.Time
		var lastError *string
		if err := rows.Scan(&eventID, &processedAt, &lastError); err != nil {
			t.Logf("scan inbox row failed: %v", err)
			continue
		}
		var processed string
		if processedAt != nil {
			processed = processedAt.UTC().Format(time.RFC3339Nano)
		}
		var lastErr string
		if lastError != nil {
			lastErr = *lastError
		}
		t.Logf("inbox_event event_id=%s processed_at=%s last_error=%s", eventID, processed, lastErr)
	}
}

func boolPtr(v bool) *bool { return &v }

func ensureAuthSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
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

func applyMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := findMigrationsDir(t)
	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	paths := make([]string, 0, len(files))
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

func findMigrationsDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for dir != "" && dir != "/" {
		candidate := filepath.Join(dir, "migrations")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}

	t.Fatalf("migrations directory not found from working directory")
	return ""
}
