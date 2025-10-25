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
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/tasks/outbox"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPublisherTaskIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(t, ctx)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(t, ctx, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard))

	srv := pstest.NewServer()
	t.Cleanup(func() { srv.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = srv.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	enableMetrics := true
	cfg := gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		MeterName:        "kratos-template.gcpubsub.test",
		EmulatorEndpoint: srv.Addr,
	}

	component, cleanupPub, err := gcpubsub.NewComponent(ctx, cfg, gcpubsub.Dependencies{
		Logger: log.NewStdLogger(io.Discard),
	})
	require.NoError(t, err)
	defer cleanupPub()

	publisher := gcpubsub.ProvidePublisher(component)

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("kratos-template.outbox.test")

	taskCfg := outbox.Config{
		BatchSize:      4,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		MaxAttempts:    3,
		PublishTimeout: time.Second,
		Workers:        2,
		LockTTL:        time.Second,
	}

	task := outbox.NewPublisherTask(repo, publisher, taskCfg, log.NewStdLogger(io.Discard), meter)

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
	go func() {
		errCh <- task.Run(runCtx)
	}()

	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		var attempts int32
		queryErr := pool.QueryRow(ctx,
			`SELECT published_at, delivery_attempts FROM catalog.outbox_events WHERE event_id = $1`,
			eventID).Scan(&publishedAt, &attempts)
		if queryErr != nil {
			return false
		}
		return publishedAt.Valid && attempts == 1
	}, 5*time.Second, 50*time.Millisecond, "outbox event should be marked as published")

	msgs := srv.Messages()
	require.Len(t, msgs, 1)
	require.Equal(t, topicName, msgs[0].Topic)
	require.Equal(t, payload, msgs[0].Data)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	var successCount int64
	var backlogSnapshot []int64

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if metric.Name == "outbox_publish_success_total" {
					for _, dp := range data.DataPoints {
						successCount += dp.Value
					}
				}
			case metricdata.Gauge[int64]:
				if metric.Name == "outbox_backlog" {
					for _, dp := range data.DataPoints {
						backlogSnapshot = append(backlogSnapshot, dp.Value)
					}
				}
			}
		}
	}

	require.Equal(t, int64(1), successCount, "success counter should record single publish")
	if len(backlogSnapshot) > 0 {
		require.Equal(t, int64(0), backlogSnapshot[len(backlogSnapshot)-1], "backlog gauge should settle at zero")
	}

	cancel()

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			return err == nil || errors.Is(err, context.Canceled)
		default:
			return false
		}
	}, time.Second, 20*time.Millisecond, "publisher task should exit after cancel")
}

func TestPublisherTaskIntegration_PublishFailureReschedules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(t, ctx)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(t, ctx, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard))

	srv := pstest.NewServer()
	t.Cleanup(func() { srv.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"
	// 故意不创建 topic，触发 Publish 时的 NotFound 错误，验证重试与 backlog 行为。

	enableMetrics := true
	cfg := gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		MeterName:        "kratos-template.gcpubsub.test",
		EmulatorEndpoint: srv.Addr,
	}

	component, cleanupPub, err := gcpubsub.NewComponent(ctx, cfg, gcpubsub.Dependencies{
		Logger: log.NewStdLogger(io.Discard),
	})
	require.NoError(t, err)
	defer cleanupPub()

	publisher := gcpubsub.ProvidePublisher(component)

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("kratos-template.outbox.test")

	taskCfg := outbox.Config{
		BatchSize:      4,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		MaxAttempts:    3,
		PublishTimeout: 100 * time.Millisecond,
		Workers:        1,
		LockTTL:        2 * time.Second,
	}

	task := outbox.NewPublisherTask(repo, publisher, taskCfg, log.NewStdLogger(io.Discard), meter)

	eventID := uuid.New()
	aggregateID := uuid.New()

	require.NoError(t, repo.Enqueue(ctx, nil, repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   aggregateID,
		EventType:     "catalog.video.created",
		Payload:       []byte(`{"video_id":"` + aggregateID.String() + `"}`),
		Headers: map[string]string{
			"schema_version": "v1",
		},
		AvailableAt: time.Now().UTC(),
	}))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- task.Run(runCtx)
	}()

	// 等待 Publish 失败后被重调度（delivery_attempts 增加，last_error 写入，available_at 推进）。
	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		var attempts int32
		var lastErr pgtype.Text
		var availableAt pgtype.Timestamptz
		queryErr := pool.QueryRow(ctx,
			`SELECT published_at, delivery_attempts, last_error, available_at FROM catalog.outbox_events WHERE event_id = $1`,
			eventID).Scan(&publishedAt, &attempts, &lastErr, &availableAt)
		if queryErr != nil {
			return false
		}
		return !publishedAt.Valid && attempts >= 1 && lastErr.Valid && availableAt.Time.After(time.Now().Add(-time.Second))
	}, 5*time.Second, 50*time.Millisecond, "outbox event should be rescheduled after publish failure")

	// backlog 仍应为 1，验证指标记录。
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	var failureCount int64
	var backlogSnapshot []int64
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if metric.Name == "outbox_publish_failure_total" {
					for _, dp := range data.DataPoints {
						failureCount += dp.Value
					}
				}
			case metricdata.Gauge[int64]:
				if metric.Name == "outbox_backlog" {
					for _, dp := range data.DataPoints {
						backlogSnapshot = append(backlogSnapshot, dp.Value)
					}
				}
			}
		}
	}

	require.GreaterOrEqual(t, failureCount, int64(1), "failure counter should record publish attempts")
	if len(backlogSnapshot) > 0 {
		require.Equal(t, int64(1), backlogSnapshot[len(backlogSnapshot)-1], "backlog gauge should remain at 1 after failure")
	}

	cancel()

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			return err == nil || errors.Is(err, context.Canceled)
		default:
			return false
		}
	}, time.Second, 20*time.Millisecond, "publisher task should exit after cancel")
}

func TestPublisherTaskIntegration_FailureThenRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dsn, terminate := startPostgres(t, ctx)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(t, ctx, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard))

	srv := pstest.NewServer()
	t.Cleanup(func() { srv.Close() })

	projectID := "test-project"
	topicID := "catalog-video-events"
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)

	enableMetrics := true
	cfg := gcpubsub.Config{
		ProjectID:        projectID,
		TopicID:          topicID,
		EnableLogging:    boolPtr(false),
		EnableMetrics:    &enableMetrics,
		MeterName:        "kratos-template.gcpubsub.test",
		EmulatorEndpoint: srv.Addr,
	}

	component, cleanupPub, err := gcpubsub.NewComponent(ctx, cfg, gcpubsub.Dependencies{
		Logger: log.NewStdLogger(io.Discard),
	})
	require.NoError(t, err)
	defer cleanupPub()

	publisher := gcpubsub.ProvidePublisher(component)

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("kratos-template.outbox.test")

	taskCfg := outbox.Config{
		BatchSize:      4,
		TickInterval:   50 * time.Millisecond,
		InitialBackoff: 20 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		MaxAttempts:    5,
		PublishTimeout: 100 * time.Millisecond,
		Workers:        1,
		LockTTL:        2 * time.Second,
	}

	task := outbox.NewPublisherTask(repo, publisher, taskCfg, log.NewStdLogger(io.Discard), meter)

	eventID := uuid.New()
	aggregateID := uuid.New()

	require.NoError(t, repo.Enqueue(ctx, nil, repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   aggregateID,
		EventType:     "catalog.video.created",
		Payload:       []byte(`{"video_id":"` + aggregateID.String() + `"}`),
		Headers: map[string]string{
			"schema_version": "v1",
		},
		AvailableAt: time.Now().UTC(),
	}))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- task.Run(runCtx)
	}()

	// Step 1: 先确认失败已发生（事件被重调度，未发布）。
	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		var attempts int32
		queryErr := pool.QueryRow(ctx,
			`SELECT published_at, delivery_attempts FROM catalog.outbox_events WHERE event_id = $1`,
			eventID).Scan(&publishedAt, &attempts)
		if queryErr != nil {
			return false
		}
		return !publishedAt.Valid && attempts >= 1
	}, 5*time.Second, 50*time.Millisecond, "event should fail at least once before recovery")

	// Step 2: 创建 Topic，让后续重试可以成功。
	_, err = srv.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	// Step 3: 等到事件被成功发布。
	require.Eventually(t, func() bool {
		var publishedAt pgtype.Timestamptz
		var attempts int32
		queryErr := pool.QueryRow(ctx,
			`SELECT published_at, delivery_attempts FROM catalog.outbox_events WHERE event_id = $1`,
			eventID).Scan(&publishedAt, &attempts)
		if queryErr != nil {
			return false
		}
		return publishedAt.Valid && attempts >= 1
	}, 10*time.Second, 100*time.Millisecond, "event should be published after topic creation")

	// Step 4: 检查消息确实被发送。
	require.Eventually(t, func() bool {
		msgs := srv.Messages()
		return len(msgs) >= 1 && string(msgs[len(msgs)-1].Data) == `{"video_id":"`+aggregateID.String()+`"}`
	}, 2*time.Second, 50*time.Millisecond, "pstest should receive published message")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	var successCount int64
	var failureCount int64
	var backlogSnapshot []int64

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				switch metric.Name {
				case "outbox_publish_success_total":
					for _, dp := range data.DataPoints {
						successCount += dp.Value
					}
				case "outbox_publish_failure_total":
					for _, dp := range data.DataPoints {
						failureCount += dp.Value
					}
				}
			case metricdata.Gauge[int64]:
				if metric.Name == "outbox_backlog" {
					for _, dp := range data.DataPoints {
						backlogSnapshot = append(backlogSnapshot, dp.Value)
					}
				}
			}
		}
	}

	require.GreaterOrEqual(t, failureCount, int64(1), "failure counter should capture initial retries")
	require.GreaterOrEqual(t, successCount, int64(1), "success counter should capture eventual publish")
	if len(backlogSnapshot) > 0 {
		require.Equal(t, int64(0), backlogSnapshot[len(backlogSnapshot)-1], "backlog should drain after success")
	}

	cancel()

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			return err == nil || errors.Is(err, context.Canceled)
		default:
			return false
		}
	}, time.Second, 20*time.Millisecond, "publisher task should exit after cancel")
}

func startPostgres(t *testing.T, ctx context.Context) (string, func()) {
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
		t.Skipf("skip outbox publisher integration test: cannot start postgres container: %v", err)
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

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := filepath.Join("..", "..", "..", "..", "migrations")
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

func boolPtr(v bool) *bool {
	return &v
}
