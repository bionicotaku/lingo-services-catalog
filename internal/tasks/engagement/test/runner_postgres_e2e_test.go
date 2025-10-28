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
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
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

	logger := log.NewStdLogger(io.Discard)
	repo := repositories.NewVideoUserStatesRepository(pool, logger)
	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })

	projectID := "test-project"
	topicID := "engagement.events"
	subscriptionID := "catalog.engagement"

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
		Repository: repo,
		TxManager:  txMgr,
		Logger:     logger,
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

	baseTime := time.Now().UTC().Add(-5 * time.Minute)
	like := true
	payload1, err := json.Marshal(engagement.Event{
		UserID:     userID.String(),
		VideoID:    videoID.String(),
		HasLiked:   &like,
		OccurredAt: baseTime,
		Version:    engagement.EventVersion,
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, gcpubsub.Message{Data: payload1})
	require.NoError(t, err)

	bookmarked := true
	payload2, err := json.Marshal(engagement.Event{
		UserID:        userID.String(),
		VideoID:       videoID.String(),
		HasBookmarked: &bookmarked,
		OccurredAt:    baseTime.Add(2 * time.Minute),
		Version:       engagement.EventVersion,
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, gcpubsub.Message{Data: payload2})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		state, getErr := repo.Get(ctx, nil, userID, videoID)
		if getErr != nil || state == nil {
			return false
		}
	return state.HasLiked && state.HasBookmarked && state.OccurredAt.Equal(baseTime.Add(2*time.Minute))
}, 5*time.Second, 50*time.Millisecond, "video_user_engagements_projection not updated")

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
