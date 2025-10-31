package uploads_test

import (
	"context"
	"encoding/json"
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
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/uploads"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type uploadsRunnerEnv struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	txMgr      txmanager.Manager
	publisher  gcpubsub.Publisher
	subscriber gcpubsub.Subscriber
	runner     *uploads.Runner
	logger     log.Logger
	cancel     context.CancelFunc
	errCh      chan error
	cleanup    func()
}

func newUploadsRunnerEnv(t *testing.T) *uploadsRunnerEnv {
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

	uploadRepo := repositories.NewUploadRepository(pool, logger)
	videoRepo := repositories.NewVideoRepository(pool, logger)
	inboxRepo := repositories.NewInboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})
	outboxRepo := repositories.NewOutboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})

	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	lifecycle := services.NewLifecycleWriter(videoRepo, outboxRepo, txMgr, logger)

	server := pstest.NewServer()

	projectID := "test-project"
	topicID := "gcs.uploads"
	subscriptionID := "catalog.uploads"

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

	runner, err := uploads.NewRunner(uploads.RunnerParams{
		Subscriber: subscriber,
		InboxRepo:  inboxRepo,
		UploadRepo: uploadRepo,
		Lifecycle:  lifecycle,
		TxManager:  txMgr,
		Logger:     logger,
		Config: outboxcfg.InboxConfig{
			SourceService:  "gcs",
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
				t.Fatalf("uploads runner stopped with error: %v", runErr)
			}
		case <-time.After(time.Second):
			t.Fatalf("uploads runner did not stop in time")
		}
		componentCleanup()
		_ = server.Close()
		terminate()
		pool.Close()
	}

	return &uploadsRunnerEnv{
		ctx:        ctx,
		pool:       pool,
		txMgr:      txMgr,
		publisher:  publisher,
		subscriber: subscriber,
		runner:     runner,
		logger:     logger,
		cancel:     cancel,
		errCh:      errCh,
		cleanup:    cleanup,
	}
}

func (e *uploadsRunnerEnv) Shutdown() {
	if e == nil {
		return
	}
	if e.cleanup != nil {
		e.cleanup()
	}
}

func TestUploadsRunner_ObjectFinalizeCreatesVideo(t *testing.T) {
	env := newUploadsRunnerEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	pool := env.pool
	publisher := env.publisher

	userID := uuid.New()
	videoID := uuid.New()
	bucket := "media-test"
	objectName := fmt.Sprintf("raw_videos/%s/%s", userID.String(), videoID.String())
	contentType := "video/mp4"
	expectedSize := int64(4 * 1024 * 1024)

	_, err := pool.Exec(ctx, `insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing`, userID, "upload-runner@test.local")
	require.NoError(t, err)

	md5Hex := "d41d8cd98f00b204e9800998ecf8427e"
	md5Base64 := "1B2M2Y8AsgTpgAmY7PhCfg=="
	now := time.Now().UTC()

	_, err = pool.Exec(ctx, `
        insert into catalog.uploads (
            video_id,
            user_id,
            bucket,
            object_name,
            content_type,
            expected_size,
            size_bytes,
            content_md5,
            title,
            description,
            signed_url,
            signed_url_expires_at,
            status,
            created_at,
            updated_at
        ) values (
            $1,$2,$3,$4,$5,$6,0,$7,'Runner Test','Integration flow','https://signed.example',$8,'uploading',$9,$9
        )
    `, videoID, userID, bucket, objectName, contentType, expectedSize, md5Hex, now.Add(10*time.Minute), now)
	require.NoError(t, err)

	payload := map[string]any{
		"bucket":      bucket,
		"name":        objectName,
		"generation":  "1",
		"size":        fmt.Sprintf("%d", expectedSize),
		"contentType": contentType,
		"md5Hash":     md5Base64,
		"crc32c":      "AAAAAA==",
		"etag":        "etag-value",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	eventID := fmt.Sprintf("%s/%s#%s", bucket, objectName, "1")

	attrs := map[string]string{
		"event_type":       "OBJECT_FINALIZE",
		"event_id":         eventID,
		"bucketId":         bucket,
		"objectId":         objectName,
		"objectGeneration": "1",
		"aggregate_type":   "upload",
		"aggregate_id":     videoID.String(),
		"schema_version":   "v1",
	}

	_, err = publisher.Publish(ctx, gcpubsub.Message{Data: data, Attributes: attrs})
	require.NoError(t, err)

	upload := waitForUploadSession(ctx, t, pool, videoID, 20*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "completed" && row.SizeBytes == expectedSize && row.MD5Hex == md5Hex
	})
	require.NotNil(t, upload)

	video := waitForVideoRecord(ctx, t, pool, videoID, 20*time.Second, func(row videoRecord) bool {
		return row.RawFileReference == fmt.Sprintf("gs://%s/%s", bucket, objectName) && row.Status == string(po.VideoStatusProcessing)
	})
	require.NotNil(t, video)

	events := countVideoCreatedEvents(ctx, t, pool, videoID)
	require.EqualValues(t, 1, events)

	// Publish the same finalize event again to ensure idempotency.
	_, err = publisher.Publish(ctx, gcpubsub.Message{Data: data, Attributes: attrs})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	uploadAgain := waitForUploadSession(ctx, t, pool, videoID, 5*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "completed" && row.SizeBytes == expectedSize
	})
	require.NotNil(t, uploadAgain)

	eventsAfter := countVideoCreatedEvents(ctx, t, pool, videoID)
	require.EqualValues(t, 1, eventsAfter)
}

func TestUploadsRunner_MD5MismatchMarksFailed(t *testing.T) {
	env := newUploadsRunnerEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	pool := env.pool
	publisher := env.publisher

	userID := uuid.New()
	videoID := uuid.New()
	bucket := "media-test"
	objectName := fmt.Sprintf("raw_videos/%s/%s", userID.String(), videoID.String())
	contentType := "video/mp4"
	expectedSize := int64(2 * 1024 * 1024)

	_, err := pool.Exec(ctx, `
        insert into catalog.uploads (
            video_id,
            user_id,
            bucket,
            object_name,
            content_type,
            expected_size,
            size_bytes,
            content_md5,
            title,
            description,
            signed_url,
            signed_url_expires_at,
            status,
            created_at,
            updated_at
        ) values (
            $1,$2,$3,$4,$5,$6,0,$7,'Runner Test','MD5 mismatch','https://signed.example',$8,'uploading',$9,$9
        )
    `, videoID, userID, bucket, objectName, contentType, expectedSize, "d41d8cd98f00b204e9800998ecf8427e", time.Now().Add(5*time.Minute), time.Now())
	require.NoError(t, err)

	payload := map[string]any{
		"bucket":      bucket,
		"name":        objectName,
		"generation":  "3",
		"size":        fmt.Sprintf("%d", expectedSize),
		"contentType": contentType,
		"md5Hash":     "XUFAKrxLKna5cZ2REBfFkg==", // md5("hello")
		"crc32c":      "AAAAAA==",
		"etag":        "etag-mismatch",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	attrs := map[string]string{
		"event_type":       "OBJECT_FINALIZE",
		"event_id":         fmt.Sprintf("%s/%s#3", bucket, objectName),
		"bucketId":         bucket,
		"objectId":         objectName,
		"objectGeneration": "3",
		"aggregate_type":   "upload",
		"aggregate_id":     videoID.String(),
		"schema_version":   "v1",
	}

	_, err = publisher.Publish(ctx, gcpubsub.Message{Data: data, Attributes: attrs})
	require.NoError(t, err)

	upload := waitForUploadSession(ctx, t, pool, videoID, 10*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "failed" && row.ErrorCode == "MD5_MISMATCH"
	})
	require.NotNil(t, upload)
	require.Equal(t, "MD5_MISMATCH", upload.ErrorCode)
	require.Equal(t, "", upload.MD5Hex)

	var videoCount int
	err = pool.QueryRow(ctx, `select count(*) from catalog.videos where video_id = $1`, videoID).Scan(&videoCount)
	require.NoError(t, err)
	require.Equal(t, 0, videoCount)

	events := countVideoCreatedEvents(ctx, t, pool, videoID)
	require.EqualValues(t, 0, events)
}

type uploadSessionRow struct {
	Status      string
	SizeBytes   int64
	MD5Hex      string
	Generation  string
	ContentType string
	ErrorCode   string
	ErrorMsg    string
}

type videoRecord struct {
	RawFileReference string
	Status           string
}

func waitForUploadSession(ctx context.Context, t *testing.T, pool *pgxpool.Pool, videoID uuid.UUID, timeout time.Duration, predicate func(uploadSessionRow) bool) *uploadSessionRow {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := pool.QueryRow(ctx, `select status, size_bytes, md5_hash, gcs_generation, content_type, error_code, error_message from catalog.uploads where video_id = $1`, videoID)
		var status string
		var size int64
		var md5 pgtype.Text
		var generation pgtype.Text
		var contentType pgtype.Text
		var errorCode pgtype.Text
		var errorMessage pgtype.Text
		err := row.Scan(&status, &size, &md5, &generation, &contentType, &errorCode, &errorMessage)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			t.Fatalf("scan upload session: %v", err)
		}
		item := uploadSessionRow{
			Status:      status,
			SizeBytes:   size,
			MD5Hex:      md5.String,
			Generation:  generation.String,
			ContentType: contentType.String,
			ErrorCode:   errorCode.String,
			ErrorMsg:    errorMessage.String,
		}
		if predicate == nil || predicate(item) {
			return &item
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func waitForVideoRecord(ctx context.Context, t *testing.T, pool *pgxpool.Pool, videoID uuid.UUID, timeout time.Duration, predicate func(videoRecord) bool) *videoRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := pool.QueryRow(ctx, `select raw_file_reference, status from catalog.videos where video_id = $1`, videoID)
		var record videoRecord
		if err := row.Scan(&record.RawFileReference, &record.Status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			t.Fatalf("scan video record: %v", err)
		}
		if predicate == nil || predicate(record) {
			return &record
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func countVideoCreatedEvents(ctx context.Context, t *testing.T, pool *pgxpool.Pool, videoID uuid.UUID) int64 {
	t.Helper()
	row := pool.QueryRow(ctx, `select count(*) from catalog.outbox_events where event_type = 'catalog.video.created' and aggregate_id = $1`, videoID)
	var count int64
	require.NoError(t, row.Scan(&count))
	return count
}

func boolPtr(v bool) *bool { return &v }

func ensureAuthSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `create schema if not exists auth`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
        create table if not exists auth.users (
            id uuid primary key,
            email text
        )`)
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
		t.Skipf("skip uploads runner integration: failed to start postgres container: %v", err)
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
	entries, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		paths = append(paths, filepath.Join(migrationsDir, entry.Name()))
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
