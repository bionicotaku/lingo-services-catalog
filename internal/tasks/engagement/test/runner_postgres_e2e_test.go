package engagement_test

import (
	"context"
	"encoding/json"
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
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestEngagementRunner_WithRealRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	ensureAuthSchema(ctx, t, pool)
	applyMigrations(ctx, t, pool)
	_, err = pool.Exec(ctx, `set search_path to catalog, public`)
	require.NoError(t, err)

	logger := log.NewStdLogger(io.Discard)
	repo := repositories.NewVideoUserStatesRepository(pool, logger)
	inboxRepo := repositories.NewInboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})
	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })

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
	component, cleanup, err := gcpubsub.NewComponent(ctx, gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		SubscriptionID:   subscriptionID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		EmulatorEndpoint: server.Addr,
	}, gcpubsub.Dependencies{Logger: logger})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	publisher := gcpubsub.ProvidePublisher(component)
	subscriber := gcpubsub.ProvideSubscriber(component)

	runner, err := engagement.NewRunner(engagement.RunnerParams{
		Subscriber: subscriber,
		InboxRepo:  inboxRepo,
		UserRepo:   repo,
		TxManager:  txMgr,
		Logger:     logger,
		Config: outboxcfg.InboxConfig{
			SourceService:  "profile",
			MaxConcurrency: 4,
		},
	})
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(runCtx)
	}()

	userID := uuid.New()
	videoID := uuid.New()

	_, err = pool.Exec(ctx, `insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing`, userID, "catalog-engagement@test.local")
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
	payload1, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "like",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime,
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	likeEventID, err := publishEvent(ctx, publisher, payload1, "profile.engagement.added", videoID)
	require.NoError(t, err)

	payload2, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(2 * time.Minute),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	bookmarkEventID, err := publishEvent(ctx, publisher, payload2, "profile.engagement.added", videoID)
	require.NoError(t, err)

	state := waitForState(ctx, t, repo, pool, userID, videoID, 5*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && st.HasBookmarked &&
			st.LikedOccurredAt != nil && approxEqual(*st.LikedOccurredAt, baseTime) &&
			st.BookmarkedOccurredAt != nil && approxEqual(*st.BookmarkedOccurredAt, baseTime.Add(2*time.Minute))
	})
	require.NotNil(t, state)

	payload3, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.removed",
		State:          "removed",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(4 * time.Minute),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	removeBookmarkEventID, err := publishEvent(ctx, publisher, payload3, "profile.engagement.removed", videoID)
	require.NoError(t, err)

	state = waitForState(ctx, t, repo, pool, userID, videoID, 5*time.Second, func(st *po.VideoUserState) bool {
		return st.HasLiked && !st.HasBookmarked &&
			st.LikedOccurredAt != nil && approxEqual(*st.LikedOccurredAt, baseTime) &&
			st.BookmarkedOccurredAt != nil && approxEqual(*st.BookmarkedOccurredAt, baseTime.Add(4*time.Minute))
	})
	require.NotNil(t, state)

	videoRepo := repositories.NewVideoRepository(pool, logger)
	querySvc := services.NewVideoQueryService(videoRepo, repo, txMgr, logger)
	queryCtx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: userID.String()})
	detail, _, err := querySvc.GetVideoDetail(queryCtx, videoID)
	require.NoError(t, err)
	require.True(t, detail.HasLiked)
	require.False(t, detail.HasBookmarked)

	assertInboxProcessed(ctx, t, pool, likeEventID)
	assertInboxProcessed(ctx, t, pool, bookmarkEventID)
	assertInboxProcessed(ctx, t, pool, removeBookmarkEventID)

	cancel()
	select {
	case runErr := <-errCh:
		if runErr != nil && runErr != context.Canceled {
			t.Fatalf("runner returned error: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop in time")
	}
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
			return "postgres://postgres:postgres@" + host + ":" + port.Port() + "/catalog?sslmode=disable"
		}).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skip engagement runner integration: cannot start postgres container: %v", err)
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

func publishEvent(ctx context.Context, publisher gcpubsub.Publisher, payload []byte, eventType string, videoID uuid.UUID) (uuid.UUID, error) {
	eventID := uuid.New()
	attrs := map[string]string{
		"event_id":       eventID.String(),
		"event_type":     eventType,
		"aggregate_type": "video",
		"aggregate_id":   videoID.String(),
	}
	_, err := publisher.Publish(ctx, gcpubsub.Message{Data: payload, Attributes: attrs})
	return eventID, err
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

func waitForState(ctx context.Context, t *testing.T, repo *repositories.VideoUserStatesRepository, pool *pgxpool.Pool, userID, videoID uuid.UUID, timeout time.Duration, predicate func(*po.VideoUserState) bool) *po.VideoUserState {
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

func approxEqual(a time.Time, b time.Time) bool {
	const tolerance = 50 * time.Millisecond
	if a.IsZero() || b.IsZero() {
		return false
	}
	return a.Sub(b).Abs() <= tolerance
}

func assertInboxProcessed(ctx context.Context, t *testing.T, pool *pgxpool.Pool, eventID uuid.UUID) {
	row := pool.QueryRow(ctx, `select processed_at, last_error from catalog.inbox_events where event_id = $1`, eventID)
	var processedAt *time.Time
	var lastError *string
	if err := row.Scan(&processedAt, &lastError); err != nil {
		t.Fatalf("fetch inbox event %s failed: %v", eventID, err)
	}
	if processedAt == nil {
		t.Fatalf("inbox event %s not processed", eventID)
	}
	if lastError != nil && *lastError != "" {
		t.Fatalf("inbox event %s recorded error: %s", eventID, *lastError)
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
