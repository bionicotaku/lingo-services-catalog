package projection_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/pstest"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/tasks/projection"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/protobuf/proto"
)

var inboxTestConfig = outboxcfg.InboxConfig{SourceService: "catalog", MaxConcurrency: 4}

func TestProjectionTaskIntegration_CreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	reader, restore := installTestMeterProvider()
	defer restore()

	env := newProjectionTestEnv(ctx, t)
	defer env.cleanup()

	cancel, wait := startProjectionTask(env)
	defer func() {
		cancel()
		require.NoError(t, wait())
	}()

	videoID := uuid.New()
	createdEvent := newVideoCreatedEvent(videoID, env.now())
	publishEvent(ctx, t, env.publisher, createdEvent)

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == createdEvent.GetVersion() && record.Title == createdEvent.GetCreated().GetTitle()
	}, 5*time.Second, 50*time.Millisecond, "projection row not created")

	updatedEvent := newVideoUpdatedEvent(videoID, env.now(), 2, "Advanced English", "published", "ready", "ready")
	publishEvent(ctx, t, env.publisher, updatedEvent)

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == updatedEvent.GetVersion() && record.Title == updatedEvent.GetUpdated().GetTitle()
	}, 5*time.Second, 50*time.Millisecond, "projection row not updated")

	deletedEvent := newVideoDeletedEvent(videoID, env.now(), 3)
	publishEvent(ctx, t, env.publisher, deletedEvent)

	require.Eventually(t, func() bool {
		_, err := env.projectionRepo.Get(ctx, nil, videoID)
		return errors.Is(err, pgx.ErrNoRows)
	}, 5*time.Second, 50*time.Millisecond, "projection row not deleted")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	var successCount, failureCount int64
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				switch metric.Name {
				case "projection_apply_success_total":
					for _, dp := range data.DataPoints {
						successCount += dp.Value
					}
				case "projection_apply_failure_total":
					for _, dp := range data.DataPoints {
						failureCount += dp.Value
					}
				}
			}
		}
	}
	require.GreaterOrEqual(t, successCount, int64(3))
	require.EqualValues(t, 0, failureCount)
}

func TestProjectionTaskIntegration_DuplicateEventIgnored(t *testing.T) {
	ctx := context.Background()
	_, restore := installTestMeterProvider()
	defer restore()

	env := newProjectionTestEnv(ctx, t)
	defer env.cleanup()

	cancel, wait := startProjectionTask(env)
	defer func() {
		cancel()
		require.NoError(t, wait())
	}()

	videoID := uuid.New()
	created := newVideoCreatedEvent(videoID, env.now())
	publishEvent(ctx, t, env.publisher, created)

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == created.GetVersion()
	}, 5*time.Second, 50*time.Millisecond)

	// 再次发布同一事件，预期不会重复写入。
	publishEvent(ctx, t, env.publisher, created)

	time.Sleep(200 * time.Millisecond)

	var inboxCount int
	require.NoError(t, env.pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog.inbox_events").Scan(&inboxCount))
	require.Equal(t, 1, inboxCount, "duplicate event should not create new inbox row")

	record, err := env.projectionRepo.Get(ctx, nil, videoID)
	require.NoError(t, err)
	require.Equal(t, int64(1), record.Version)
}

func TestProjectionTaskIntegration_OutOfOrderVersionIgnored(t *testing.T) {
	ctx := context.Background()
	_, restore := installTestMeterProvider()
	defer restore()

	env := newProjectionTestEnv(ctx, t)
	defer env.cleanup()

	cancel, wait := startProjectionTask(env)
	defer func() {
		cancel()
		require.NoError(t, wait())
	}()

	videoID := uuid.New()
	publishEvent(ctx, t, env.publisher, newVideoCreatedEvent(videoID, env.now()))

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == 1
	}, 5*time.Second, 50*time.Millisecond)

	publishEvent(ctx, t, env.publisher, newVideoUpdatedEvent(videoID, env.now(), 3, "Intermediate", "published", "ready", "ready"))

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == 3
	}, 5*time.Second, 50*time.Millisecond)

	// 乱序事件（版本 2）不应覆盖现有版本 3 数据。
	publishEvent(ctx, t, env.publisher, newVideoUpdatedEvent(videoID, env.now(), 2, "Basic", "ready", "processing", "processing"))

	time.Sleep(200 * time.Millisecond)

	record, err := env.projectionRepo.Get(ctx, nil, videoID)
	require.NoError(t, err)
	require.Equal(t, int64(3), record.Version, "out-of-order event should not downgrade version")
	require.Equal(t, "Intermediate", record.Title)
}

func TestProjectionTaskIntegration_ExactlyOnceConfig(t *testing.T) {
	ctx := context.Background()
	_, restore := installTestMeterProvider()
	defer restore()

	env := newProjectionTestEnv(ctx, t, projectionTestConfig{ExactlyOnce: true})
	defer env.cleanup()

	cancel, wait := startProjectionTask(env)
	defer func() {
		cancel()
		require.NoError(t, wait())
	}()

	videoID := uuid.New()
	publishEvent(ctx, t, env.publisher, newVideoCreatedEvent(videoID, env.now()))

	require.Eventually(t, func() bool {
		record, err := env.projectionRepo.Get(ctx, nil, videoID)
		if err != nil {
			return false
		}
		return record.Version == 1
	}, 5*time.Second, 50*time.Millisecond)
}

// --- helpers ----------------------------------------------------------------

type projectionTestEnv struct {
	task           *projection.Task
	publisher      gcpubsub.Publisher
	projectionRepo *repositories.VideoProjectionRepository
	pool           *pgxpool.Pool
	cleanup        func()
	now            func() time.Time

	server *pstest.Server
}

type projectionTestConfig struct {
	ExactlyOnce bool
}

func newProjectionTestEnv(ctx context.Context, t *testing.T, cfgOpt ...projectionTestConfig) projectionTestEnv {
	t.Helper()

	options := projectionTestConfig{}
	if len(cfgOpt) > 0 {
		options = cfgOpt[0]
	}

	dsn, terminate := startPostgres(ctx, t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	applyMigrations(ctx, t, pool)

	logger := log.NewStdLogger(io.Discard)

	txCfg := txmanager.Config{
		DefaultIsolation: "read_committed",
		DefaultTimeout:   3 * time.Second,
		LockTimeout:      0,
		MaxRetries:       0,
	}
	txManager, err := txmanager.NewManager(pool, txCfg, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	inboxRepo := repositories.NewInboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})
	projectionRepo := repositories.NewVideoProjectionRepository(pool, logger)

	server := pstest.NewServer()

	projectID := "test-project"
	topicID := "catalog.video.events"
	subscriptionID := "catalog.video.events.catalog-reader"
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	subName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)

	_, err = server.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	_, err = server.GServer.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:                  subName,
		Topic:                 topicName,
		AckDeadlineSeconds:    60,
		EnableMessageOrdering: true,
	})
	require.NoError(t, err)

	cfg := gcpubsub.Config{
		ProjectID:          projectID,
		TopicID:            topicID,
		SubscriptionID:     subscriptionID,
		OrderingKeyEnabled: boolPtr(true),
		EnableLogging:      boolPtr(false),
		EnableMetrics:      boolPtr(true),
		EmulatorEndpoint:   server.Addr,
		Receive: gcpubsub.ReceiveConfig{
			NumGoroutines:          4,
			MaxOutstandingMessages: 100,
			MaxOutstandingBytes:    64 << 20,
			MaxExtension:           60 * time.Second,
			MaxExtensionPeriod:     10 * time.Minute,
		},
		ExactlyOnceDelivery: options.ExactlyOnce,
	}
	component, cleanupComponent, err := gcpubsub.NewComponent(ctx, cfg, gcpubsub.Dependencies{
		Logger: logger,
	})
	require.NoError(t, err)

	publisher := gcpubsub.ProvidePublisher(component)
	subscriber := gcpubsub.ProvideSubscriber(component)

	task := projection.NewTask(subscriber, inboxRepo, projectionRepo, txManager, logger, inboxTestConfig)
	task.WithClock(time.Now)

	cleanup := func() {
		cleanupComponent()
		terminate()
		pool.Close()
		server.Close()
	}

	return projectionTestEnv{
		task:           task,
		publisher:      publisher,
		projectionRepo: projectionRepo,
		pool:           pool,
		cleanup:        cleanup,
		now:            time.Now,
		server:         server,
	}
}

func startProjectionTask(env projectionTestEnv) (context.CancelFunc, func() error) {
	runCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	var runErr error
	go func() {
		defer wg.Done()
		if err := env.task.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			runErr = err
		}
	}()

	return cancel, func() error {
		wg.Wait()
		return runErr
	}
}

func publishEvent(ctx context.Context, t *testing.T, publisher gcpubsub.Publisher, evt *videov1.Event) {
	t.Helper()
	data, err := proto.Marshal(evt)
	require.NoError(t, err)

	msg := gcpubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"event_id":       evt.GetEventId(),
			"event_type":     evt.GetEventType().String(),
			"aggregate_id":   evt.GetAggregateId(),
			"aggregate_type": evt.GetAggregateType(),
			"version":        fmt.Sprintf("%d", evt.GetVersion()),
		},
		OrderingKey: evt.GetAggregateId(),
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = publisher.Publish(ctx, msg)
	require.NoError(t, err)
}

func newVideoCreatedEvent(videoID uuid.UUID, now time.Time) *videov1.Event {
	description := "lesson plan"
	duration := int64(120_000_000)
	return &videov1.Event{
		EventId:       uuid.NewString(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_CREATED,
		AggregateId:   videoID.String(),
		AggregateType: "video",
		Version:       1,
		OccurredAt:    now.Format(time.RFC3339Nano),
		Payload: &videov1.Event_Created{
			Created: &videov1.Event_VideoCreated{
				VideoId:        videoID.String(),
				UploaderId:     uuid.NewString(),
				Title:          "English Basics",
				Description:    &description,
				DurationMicros: &duration,
				Status:         "ready",
				MediaStatus:    "ready",
				AnalysisStatus: "ready",
				Version:        1,
				OccurredAt:     now.Format(time.RFC3339Nano),
			},
		},
	}
}

func newVideoUpdatedEvent(videoID uuid.UUID, now time.Time, version int64, title string, status string, media string, analysis string) *videov1.Event {
	titlePtr := title
	statusPtr := status
	mediaPtr := media
	analysisPtr := analysis
	return &videov1.Event{
		EventId:       uuid.NewString(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
		AggregateId:   videoID.String(),
		AggregateType: "video",
		Version:       version,
		OccurredAt:    now.Format(time.RFC3339Nano),
		Payload: &videov1.Event_Updated{
			Updated: &videov1.Event_VideoUpdated{
				VideoId:        videoID.String(),
				Version:        version,
				OccurredAt:     now.Format(time.RFC3339Nano),
				Title:          &titlePtr,
				Status:         &statusPtr,
				MediaStatus:    &mediaPtr,
				AnalysisStatus: &analysisPtr,
			},
		},
	}
}

func newVideoDeletedEvent(videoID uuid.UUID, now time.Time, version int64) *videov1.Event {
	return &videov1.Event{
		EventId:       uuid.NewString(),
		EventType:     videov1.EventType_EVENT_TYPE_VIDEO_DELETED,
		AggregateId:   videoID.String(),
		AggregateType: "video",
		Version:       version,
		OccurredAt:    now.Format(time.RFC3339Nano),
		Payload: &videov1.Event_Deleted{
			Deleted: &videov1.Event_VideoDeleted{
				VideoId:    videoID.String(),
				Version:    version,
				OccurredAt: now.Format(time.RFC3339Nano),
			},
		},
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
		WaitingFor: wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
			return fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skip projection integration test: cannot start postgres container: %v", err)
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

	migrations := []string{
		"migrations/001_init_catalog_schema.sql",
		"migrations/002_create_catalog_event_tables.sql",
		"migrations/003_create_catalog_videos_table.sql",
		"migrations/004_create_catalog_video_projection.sql",
	}

	for _, path := range migrations {
		sqlBytes, err := os.ReadFile(filepath.Join("../../..", path))
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, err, "apply migration %s", path)
	}
}

func installTestMeterProvider() (*sdkmetric.ManualReader, func()) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	return reader, func() {
		otel.SetMeterProvider(prev)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
