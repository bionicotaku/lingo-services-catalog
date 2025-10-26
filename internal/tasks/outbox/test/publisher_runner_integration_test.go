package outbox_test

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
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	outboxpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var defaultOutboxConfig = outboxcfg.Config{
	Schema: "catalog",
	Inbox: outboxcfg.InboxConfig{
		SourceService:  "catalog",
		MaxConcurrency: 4,
	},
}

func TestPublisherRunner_SuccessfulPublish(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard), defaultOutboxConfig)

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = server.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	_, cleanupPub, publisher := newTestPublisher(ctx, t, server, projectID, topicID)
	defer cleanupPub()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("lingo-services-catalog.outbox.test")

	runner := newPublisherRunner(t, repo, publisher, meter, outboxcfg.PublisherConfig{
		BatchSize:      4,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		MaxAttempts:    3,
		PublishTimeout: time.Second,
		Workers:        2,
		LockTTL:        time.Second,
	})

	eventID := uuid.New()
	aggregateID := uuid.New()
	payload := []byte(`{"video_id":"` + aggregateID.String() + `"}`)

	require.NoError(t, repo.Enqueue(ctx, nil, repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   aggregateID,
		EventType:     "catalog.video.created",
		Payload:       payload,
		Headers: map[string]string{
			"schema_version": "v1",
		},
		AvailableAt: time.Now().UTC(),
	}))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(runCtx) }()

	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		var attempts int32
		queryErr := pool.QueryRow(ctx, `
			SELECT published_at, delivery_attempts
			FROM catalog.outbox_events
			WHERE event_id = $1`, eventID).Scan(&publishedAt, &attempts)
		if queryErr != nil {
			return false
		}
		return publishedAt.Valid && attempts == 1
	}, 5*time.Second, 50*time.Millisecond)

	msgs := server.Messages()
	require.Len(t, msgs, 1)
	require.Equal(t, topicName, msgs[0].Topic)

	cancel()
	select {
	case err := <-errCh:
		require.True(t, err == nil || errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("runner did not stop in time")
	}
}

func TestPublisherRunner_RetryOnFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard), defaultOutboxConfig)

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"

	_, cleanupPub, publisher := newTestPublisher(ctx, t, server, projectID, topicID)
	defer cleanupPub()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("lingo-services-catalog.outbox.test")

	runner := newPublisherRunner(t, repo, publisher, meter, outboxcfg.PublisherConfig{
		BatchSize:      2,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 25 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		MaxAttempts:    2,
		PublishTimeout: 100 * time.Millisecond,
		Workers:        1,
		LockTTL:        2 * time.Second,
	})

	eventID := uuid.New()
	aggregateID := uuid.New()
	require.NoError(t, repo.Enqueue(ctx, nil, repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   aggregateID,
		EventType:     "catalog.video.created",
		Payload:       []byte(`{}`),
		AvailableAt:   time.Now().UTC(),
	}))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(runCtx) }()

	require.Eventually(t, func() bool {
		var attempts int32
		queryErr := pool.QueryRow(ctx, `
			SELECT delivery_attempts
			FROM catalog.outbox_events
			WHERE event_id = $1`, eventID).Scan(&attempts)
		return queryErr == nil && attempts >= 1
	}, 3*time.Second, 50*time.Millisecond)

	var publishedAt pgtype.Timestamptz
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT published_at FROM catalog.outbox_events WHERE event_id = $1`, eventID).Scan(&publishedAt))
	require.False(t, publishedAt.Valid, "event should not be published without topic")

	cancel()
	select {
	case err := <-errCh:
		require.True(t, err == nil || errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("runner did not stop in time")
	}
}

func TestPublisherRunner_RecoveryAfterTopicCreated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard), defaultOutboxConfig)

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"

	component, cleanupPub, publisher := newTestPublisher(ctx, t, server, projectID, topicID)
	defer cleanupPub()
	t.Cleanup(func() { _ = component })

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("lingo-services-catalog.outbox.test")

	runner := newPublisherRunner(t, repo, publisher, meter, outboxcfg.PublisherConfig{
		BatchSize:      2,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 25 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		MaxAttempts:    5,
		PublishTimeout: 150 * time.Millisecond,
		Workers:        1,
		LockTTL:        2 * time.Second,
	})

	eventID := uuid.New()
	require.NoError(t, repo.Enqueue(ctx, nil, repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   uuid.New(),
		EventType:     "catalog.video.updated",
		Payload:       []byte(`{}`),
		AvailableAt:   time.Now().UTC(),
	}))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(runCtx) }()

	// 等待一次失败（topic 不存在）
	require.Eventually(t, func() bool {
		var attempts int32
		queryErr := pool.QueryRow(ctx, `
			SELECT delivery_attempts
			FROM catalog.outbox_events
			WHERE event_id = $1`, eventID).Scan(&attempts)
		return queryErr == nil && attempts >= 1
	}, 3*time.Second, 50*time.Millisecond)

	// 创建 topic，再次尝试应成功。
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = server.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		queryErr := pool.QueryRow(ctx, `
			SELECT published_at
			FROM catalog.outbox_events
			WHERE event_id = $1`, eventID).Scan(&publishedAt)
		return queryErr == nil && publishedAt.Valid
	}, 6*time.Second, 100*time.Millisecond)

	cancel()
	select {
	case err := <-errCh:
		require.True(t, err == nil || errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("runner did not stop in time")
	}
}

func newTestPublisher(ctx context.Context, t *testing.T, server *pstest.Server, projectID, topicID string) (*gcpubsub.Component, func(), gcpubsub.Publisher) {
	t.Helper()

	enableMetrics := true
	cfg := gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		MeterName:        "lingo-services-catalog.gcpubsub.test",
		EmulatorEndpoint: server.Addr,
	}

	component, cleanup, err := gcpubsub.NewComponent(ctx, cfg, gcpubsub.Dependencies{
		Logger: log.NewStdLogger(io.Discard),
	})
	require.NoError(t, err)

	publisher := gcpubsub.ProvidePublisher(component)
	return component, cleanup, publisher
}

func newPublisherRunner(t *testing.T, repo *repositories.OutboxRepository, publisher gcpubsub.Publisher, meter metricapi.Meter, cfg outboxcfg.PublisherConfig) *outboxpublisher.Runner {
	t.Helper()

	if cfg.BatchSize == 0 {
		cfg.BatchSize = 1
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 100 * time.Millisecond
	}
	logging := false
	metrics := true
	cfg.LoggingEnabled = &logging
	cfg.MetricsEnabled = &metrics

	runner, err := outboxpublisher.NewRunner(outboxpublisher.RunnerParams{
		Store:     repo.Shared(),
		Publisher: publisher,
		Config:    cfg,
		Logger:    log.NewStdLogger(io.Discard),
		Meter:     meter,
	})
	require.NoError(t, err)
	return runner
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
		WaitingFor: wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
			return fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skip outbox integration tests: cannot start postgres container: %v", err)
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

func boolPtr(v bool) *bool {
	b := v
	return &b
}
