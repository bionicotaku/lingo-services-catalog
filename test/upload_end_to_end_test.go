package test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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

func TestUploadEndToEnd(t *testing.T) {
	t.Parallel()

	env := newUploadE2EEnv(t)
	defer env.Shutdown()

	ctx := env.ctx

	// 1. 客户端准备要上传的文件并计算 MD5。
	content := []byte(strings.Repeat("learning-app-demo-", 128)) // 2KB 样例
	tmpFile := filepath.Join(t.TempDir(), "sample.mp4")
	require.NoError(t, os.WriteFile(tmpFile, content, 0o600))

	sum := md5.Sum(content)
	md5Hex := fmt.Sprintf("%x", sum)
	md5Base64 := base64.StdEncoding.EncodeToString(sum[:])

	userID := uuid.New()
	require.NoError(t, env.bootstrapAuthUser(ctx, userID))

	// 2. 客户端调用 InitResumableUpload。
	req := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       int64(len(content)),
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 90,
		Title:           "E2E Demo",
		Description:     "Demonstrate end-to-end upload flow",
	}
	res, err := env.uploadSvc.InitResumableUpload(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.Reused)
	require.NotZero(t, res.Session.VideoID)
	require.NotEmpty(t, res.ResumableInitURL)

	// 3. 移动端按照签名 URL 发起 Resumable 会话，获取 Session URI 并上传字节流。
	sessionURI := env.fakeGCS.InitiateResumableUpload(t, res.ResumableInitURL, "video/mp4")
	status := env.fakeGCS.UploadContent(t, sessionURI, content)
	require.Equal(t, http.StatusOK, status)

	// 4. GCS 触发 OBJECT_FINALIZE，Catalog Runner 通过 Pub/Sub 收到消息。
	finalize := map[string]string{
		"bucket":      env.bucket,
		"name":        res.Session.ObjectName,
		"generation":  "1",
		"size":        fmt.Sprintf("%d", len(content)),
		"contentType": "video/mp4",
		"md5Hash":     md5Base64,
		"crc32c":      "AAAAAA==",
		"etag":        "test-etag",
	}
	env.publishFinalize(ctx, finalize, res.Session.VideoID)

	// 5. 等待 Runner 完成处理：uploads 表状态、videos 主表记录、outbox 事件。
	uploadRow := env.waitForUploadSession(ctx, t, res.Session.VideoID, 15*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "completed" && row.MD5Hex == md5Hex && row.SizeBytes == int64(len(content))
	})
	require.NotNil(t, uploadRow, "expected uploads session to be marked completed")
	require.Equal(t, md5Hex, uploadRow.MD5Hex)
	require.Equal(t, "", uploadRow.ErrorCode)

	videoRow := env.waitForVideoRecord(ctx, t, res.Session.VideoID, 15*time.Second, func(row videoRecord) bool {
		return strings.HasPrefix(row.RawFileReference, "gs://"+env.bucket+"/") && row.Status == string(po.VideoStatusProcessing)
	})
	require.NotNil(t, videoRow, "video record not materialised")

	eventCount := env.countOutboxEvents(ctx, t, res.Session.VideoID)
	require.EqualValues(t, 1, eventCount)

	// 二次发送 finalize，验证幂等。
	env.publishFinalize(ctx, finalize, res.Session.VideoID)
	time.Sleep(500 * time.Millisecond)
	eventCount = env.countOutboxEvents(ctx, t, res.Session.VideoID)
	require.EqualValues(t, 1, eventCount)
}

func TestUploadEndToEnd_ConcurrentInitSingleRow(t *testing.T) {
	env := newUploadE2EEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	userID := uuid.New()
	require.NoError(t, env.bootstrapAuthUser(ctx, userID))

	payload := []byte("concurrent-upload")
	md5Sum := md5.Sum(payload)
	md5Hex := fmt.Sprintf("%x", md5Sum)

	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       int64(len(payload)),
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 45,
		Title:           "Concurrent",
		Description:     "Stress same hash",
	}

	const workers = 10
	results := make([]*services.InitResumableUploadResult, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			res, err := env.uploadSvc.InitResumableUpload(ctx, input)
			results[idx] = res
			errs[idx] = err
		}(i)
	}

	close(start)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	first := results[0]
	require.NotNil(t, first)
	for _, res := range results {
		require.NotNil(t, res)
		require.Equal(t, first.Session.VideoID, res.Session.VideoID)
	}

	var count int
	require.NoError(t, env.pool.QueryRow(ctx, `select count(*) from catalog.uploads`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestUploadEndToEnd_SessionExpiryRenewsSignedURL(t *testing.T) {
	env := newUploadE2EEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	userID := uuid.New()
	require.NoError(t, env.bootstrapAuthUser(ctx, userID))

	content := []byte(strings.Repeat("renew-flow", 128))
	md5Sum := md5.Sum(content)
	md5Hex := fmt.Sprintf("%x", md5Sum)
	md5Base64 := base64.StdEncoding.EncodeToString(md5Sum[:])

	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       int64(len(content)),
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 60,
		Title:           "Renew",
		Description:     "Signed URL refresh",
	}

	initial, err := env.uploadSvc.InitResumableUpload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, initial)

	sessionURI1 := env.fakeGCS.InitiateResumableUpload(t, initial.ResumableInitURL, "video/mp4")

	_, err = env.pool.Exec(ctx, `update catalog.uploads set signed_url_expires_at = now() - interval '2 minutes' where video_id = $1`, initial.Session.VideoID)
	require.NoError(t, err)
	env.fakeGCS.InvalidateSession(sessionURI1)

	renewed, err := env.uploadSvc.InitResumableUpload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, renewed)
	require.True(t, renewed.Reused)
	require.NotEqual(t, initial.ResumableInitURL, renewed.ResumableInitURL)

	statusOld := env.fakeGCS.UploadContent(t, sessionURI1, content)
	require.Equal(t, http.StatusPreconditionFailed, statusOld)

	sessionURI2 := env.fakeGCS.InitiateResumableUpload(t, renewed.ResumableInitURL, "video/mp4")
	statusNew := env.fakeGCS.UploadContent(t, sessionURI2, content)
	require.Equal(t, http.StatusOK, statusNew)

	finalize := map[string]string{
		"bucket":      env.bucket,
		"name":        renewed.Session.ObjectName,
		"generation":  "5",
		"size":        fmt.Sprintf("%d", len(content)),
		"contentType": "video/mp4",
		"md5Hash":     md5Base64,
		"crc32c":      "AAAAAA==",
		"etag":        "renew-etag",
	}

	env.publishFinalize(ctx, finalize, renewed.Session.VideoID)

	uploadRow := env.waitForUploadSession(ctx, t, renewed.Session.VideoID, 15*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "completed" && row.MD5Hex == md5Hex
	})
	require.NotNil(t, uploadRow)

	videoRow := env.waitForVideoRecord(ctx, t, renewed.Session.VideoID, 15*time.Second, nil)
	require.NotNil(t, videoRow)
}

func TestUploadEndToEnd_DuplicateFinalizeIdempotent(t *testing.T) {
	env := newUploadE2EEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	userID := uuid.New()
	require.NoError(t, env.bootstrapAuthUser(ctx, userID))

	content := []byte(strings.Repeat("dup-finalize", 64))
	md5Sum := md5.Sum(content)
	md5Hex := fmt.Sprintf("%x", md5Sum)
	md5Base64 := base64.StdEncoding.EncodeToString(md5Sum[:])

	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       int64(len(content)),
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 75,
		Title:           "Duplicate",
		Description:     "Duplicate finalize events",
	}

	res, err := env.uploadSvc.InitResumableUpload(ctx, input)
	require.NoError(t, err)

	sessionURI := env.fakeGCS.InitiateResumableUpload(t, res.ResumableInitURL, "video/mp4")
	require.Equal(t, http.StatusOK, env.fakeGCS.UploadContent(t, sessionURI, content))

	finalize := map[string]string{
		"bucket":      env.bucket,
		"name":        res.Session.ObjectName,
		"generation":  "9",
		"size":        fmt.Sprintf("%d", len(content)),
		"contentType": "video/mp4",
		"md5Hash":     md5Base64,
		"crc32c":      "AAAAAA==",
		"etag":        "dup-etag",
	}

	env.publishFinalize(ctx, finalize, res.Session.VideoID)

	_ = env.waitForUploadSession(ctx, t, res.Session.VideoID, 15*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "completed"
	})

	for i := 0; i < 2; i++ {
		env.publishFinalize(ctx, finalize, res.Session.VideoID)
	}

	time.Sleep(500 * time.Millisecond)

	count := env.countOutboxEvents(ctx, t, res.Session.VideoID)
	require.EqualValues(t, 1, count)
}

func TestUploadEndToEnd_MD5MismatchMarksFailed(t *testing.T) {
	env := newUploadE2EEnv(t)
	defer env.Shutdown()

	ctx := env.ctx
	userID := uuid.New()
	require.NoError(t, env.bootstrapAuthUser(ctx, userID))

	content := []byte(strings.Repeat("md5-mismatch", 80))
	md5Sum := md5.Sum(content)
	md5Hex := fmt.Sprintf("%x", md5Sum)

	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       int64(len(content)),
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 80,
		Title:           "MD5 Mismatch",
		Description:     "Expect failure",
	}

	res, err := env.uploadSvc.InitResumableUpload(ctx, input)
	require.NoError(t, err)

	sessionURI := env.fakeGCS.InitiateResumableUpload(t, res.ResumableInitURL, "video/mp4")
	require.Equal(t, http.StatusOK, env.fakeGCS.UploadContent(t, sessionURI, content))

	wrongSum := md5.Sum([]byte("mismatch-payload"))
	wrong := map[string]string{
		"bucket":      env.bucket,
		"name":        res.Session.ObjectName,
		"generation":  "4",
		"size":        fmt.Sprintf("%d", len(content)),
		"contentType": "video/mp4",
		"md5Hash":     base64.StdEncoding.EncodeToString(wrongSum[:]),
		"crc32c":      "AAAAAA==",
		"etag":        "bad-md5",
	}

	env.publishFinalize(ctx, wrong, res.Session.VideoID)

	uploadRow := env.waitForUploadSession(ctx, t, res.Session.VideoID, 10*time.Second, func(row uploadSessionRow) bool {
		return row.Status == "failed" && strings.EqualFold(row.ErrorCode, "MD5_MISMATCH")
	})
	require.NotNil(t, uploadRow)
	require.True(t, strings.EqualFold(uploadRow.ErrorCode, "MD5_MISMATCH"))

	videoRow := env.waitForVideoRecord(ctx, t, res.Session.VideoID, 2*time.Second, nil)
	require.Nil(t, videoRow)
	require.EqualValues(t, 0, env.countOutboxEvents(ctx, t, res.Session.VideoID))
}

// --- 测试环境搭建 ---

type uploadE2EEnv struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *pgxpool.Pool
	txMgr      txmanager.Manager
	bucket     string
	uploadSvc  *services.UploadService
	fakeGCS    *fakeGCSServer
	publisher  gcpubsub.Publisher
	runnerStop func()
}

func newUploadE2EEnv(t *testing.T) *uploadE2EEnv {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	dsn, terminate := startPostgres(ctx, t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	applyMigrations(ctx, t, pool)

	logger := log.NewStdLogger(io.Discard)

	uploadRepo := repositories.NewUploadRepository(pool, logger)
	videoRepo := repositories.NewVideoRepository(pool, logger)
	inboxRepo := repositories.NewInboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})
	outboxRepo := repositories.NewOutboxRepository(pool, logger, outboxcfg.Config{Schema: "catalog"})

	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	lifecycle := services.NewLifecycleWriter(videoRepo, outboxRepo, txMgr, logger)

	// 启动 pstest （Pub/Sub 模拟器）。
	server := pstest.NewServer()
	topicID := "gcs.uploads"
	subscriptionID := "catalog.uploads"
	projectID := "upload-e2e"

	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = server.GServer.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)

	subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)
	_, err = server.GServer.CreateSubscription(ctx, &pubsubpb.Subscription{Name: subscriptionName, Topic: topicName})
	require.NoError(t, err)

	enableMetrics := true
	component, cleanupComponent, err := gcpubsub.NewComponent(ctx, gcpubsub.Config{
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

	runCtx, runCancel := context.WithCancel(ctx)
	runnerErr := make(chan error, 1)
	go func() {
		runnerErr <- runner.Run(runCtx)
	}()

	stopRunner := func() {
		runCancel()
		select {
		case err := <-runnerErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("uploads runner stopped with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Log("uploads runner shutdown timeout")
		}
	}

	fakeGCS := newFakeGCSServer(t)
	signer := &fakeResumableSigner{server: fakeGCS}

	bucket := "catalog-upload-e2e"

	uploadSvc, err := services.NewUploadService(uploadRepo, signer, bucket, 15*time.Minute, logger)
	require.NoError(t, err)

	return &uploadE2EEnv{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		txMgr:     txMgr,
		bucket:    bucket,
		uploadSvc: uploadSvc,
		fakeGCS:   fakeGCS,
		publisher: publisher,
		runnerStop: func() {
			stopRunner()
			cleanupComponent()
			server.Close()
			terminate()
		},
	}
}

func (e *uploadE2EEnv) Shutdown() {
	if e == nil {
		return
	}
	if e.runnerStop != nil {
		e.runnerStop()
	}
	if e.pool != nil {
		e.pool.Close()
	}
	if e.cancel != nil {
		e.cancel()
	}
	if e.fakeGCS != nil {
		e.fakeGCS.Close()
	}
}

func (e *uploadE2EEnv) bootstrapAuthUser(ctx context.Context, userID uuid.UUID) error {
	_, err := e.pool.Exec(ctx, `create schema if not exists auth`)
	if err != nil {
		return err
	}
	_, err = e.pool.Exec(ctx, `
        create table if not exists auth.users (
            id uuid primary key,
            email text
        )
    `)
	if err != nil {
		return err
	}
	_, err = e.pool.Exec(ctx, `
        insert into auth.users (id, email)
        values ($1, $2)
        on conflict (id) do nothing
    `, userID, "e2e@test.local")
	return err
}

func (e *uploadE2EEnv) publishFinalize(ctx context.Context, payload map[string]string, videoID uuid.UUID) {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	attrs := map[string]string{
		"event_type":       "OBJECT_FINALIZE",
		"event_id":         fmt.Sprintf("%s/%s#%s", payload["bucket"], payload["name"], payload["generation"]),
		"bucketId":         payload["bucket"],
		"objectId":         payload["name"],
		"objectGeneration": payload["generation"],
		"aggregate_type":   "upload",
		"aggregate_id":     videoID.String(),
		"schema_version":   "v1",
	}
	_, err = e.publisher.Publish(ctx, gcpubsub.Message{Data: data, Attributes: attrs})
	if err != nil {
		panic(fmt.Errorf("publish finalize: %w", err))
	}
}

// --- 轮询辅助 ---

type uploadSessionRow struct {
	Status      string
	SizeBytes   int64
	MD5Hex      string
	Generation  string
	ContentType string
	ErrorCode   string
}

type videoRecord struct {
	RawFileReference string
	Status           string
}

func (e *uploadE2EEnv) waitForUploadSession(ctx context.Context, t *testing.T, videoID uuid.UUID, timeout time.Duration, predicate func(uploadSessionRow) bool) *uploadSessionRow {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := e.pool.QueryRow(ctx, `select status, size_bytes, md5_hash, gcs_generation, content_type, error_code from catalog.uploads where video_id = $1`, videoID)
		var status string
		var size int64
		var md5 pgtype.Text
		var generation pgtype.Text
		var contentType pgtype.Text
		var errorCode pgtype.Text
		err := row.Scan(&status, &size, &md5, &generation, &contentType, &errorCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			t.Fatalf("scan uploads: %v", err)
		}
		record := uploadSessionRow{
			Status:      status,
			SizeBytes:   size,
			MD5Hex:      md5.String,
			Generation:  generation.String,
			ContentType: contentType.String,
			ErrorCode:   errorCode.String,
		}
		if predicate == nil || predicate(record) {
			return &record
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (e *uploadE2EEnv) waitForVideoRecord(ctx context.Context, t *testing.T, videoID uuid.UUID, timeout time.Duration, predicate func(videoRecord) bool) *videoRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := e.pool.QueryRow(ctx, `select raw_file_reference, status from catalog.videos where video_id = $1`, videoID)
		var rec videoRecord
		if err := row.Scan(&rec.RawFileReference, &rec.Status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			t.Fatalf("scan videos: %v", err)
		}
		if predicate == nil || predicate(rec) {
			return &rec
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (e *uploadE2EEnv) countOutboxEvents(ctx context.Context, t *testing.T, videoID uuid.UUID) int64 {
	t.Helper()
	row := e.pool.QueryRow(ctx, `select count(*) from catalog.outbox_events where event_type = 'catalog.video.created' and aggregate_id = $1`, videoID)
	var count int64
	require.NoError(t, row.Scan(&count))
	return count
}

// --- Fake GCS 实现 ---

type fakeGCSServer struct {
	server  *httptest.Server
	baseURL string

	mu       sync.Mutex
	sessions map[string]gcsSession
	objects  map[string][]byte
	invalid  map[string]struct{}
}

type gcsSession struct {
	bucket      string
	objectName  string
	contentType string
}

func newFakeGCSServer(t *testing.T) *fakeGCSServer {
	t.Helper()
	f := &fakeGCSServer{
		sessions: make(map[string]gcsSession),
		objects:  make(map[string][]byte),
		invalid:  make(map[string]struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/fake-gcs/init", f.handleInit)
	mux.HandleFunc("/fake-gcs/upload/", f.handleUpload)
	server := httptest.NewServer(mux)
	f.server = server
	f.baseURL = server.URL
	return f
}

func (f *fakeGCSServer) Close() {
	if f != nil && f.server != nil {
		f.server.Close()
	}
}

func (f *fakeGCSServer) SignedURL(bucket, objectName, contentType, token string) string {
	params := url.Values{}
	params.Set("bucket", bucket)
	params.Set("object", objectName)
	params.Set("content_type", contentType)
	if token != "" {
		params.Set("sig_seq", token)
	}
	return fmt.Sprintf("%s/fake-gcs/init?%s", f.baseURL, params.Encode())
}

func (f *fakeGCSServer) InitiateResumableUpload(t *testing.T, signedURL, contentType string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, signedURL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Goog-Resumable", "start")
	req.Header.Set("X-Upload-Content-Type", contentType)
	req.Header.Set("X-Goog-If-Generation-Match", "0")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusCreated, res.StatusCode)
	location := res.Header.Get("Location")
	require.NotEmpty(t, location, "location header missing")
	return location
}

func (f *fakeGCSServer) UploadContent(t *testing.T, sessionURI string, payload []byte) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, sessionURI, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(payload)-1, len(payload)))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	return res.StatusCode
}

func (f *fakeGCSServer) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !strings.EqualFold(r.Header.Get("X-Goog-Resumable"), "start") {
		http.Error(w, "missing resumable header", http.StatusBadRequest)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	objectName := r.URL.Query().Get("object")
	contentType := r.URL.Query().Get("content_type")

	if bucket == "" || objectName == "" {
		http.Error(w, "missing bucket/object", http.StatusBadRequest)
		return
	}

	sessionID := uuid.NewString()
	f.mu.Lock()
	f.sessions[sessionID] = gcsSession{
		bucket:      bucket,
		objectName:  objectName,
		contentType: contentType,
	}
	f.mu.Unlock()

	w.Header().Set("Location", fmt.Sprintf("%s/fake-gcs/upload/%s", f.baseURL, sessionID))
	w.WriteHeader(http.StatusCreated)
}

func (f *fakeGCSServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/fake-gcs/upload/")
	f.mu.Lock()
	session, ok := f.sessions[sessionID]
	_, invalid := f.invalid[sessionID]
	f.mu.Unlock()
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if invalid {
		http.Error(w, "session expired", http.StatusPreconditionFailed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	f.mu.Lock()
	f.objects[session.objectName] = body
	f.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (f *fakeGCSServer) InvalidateSession(sessionURI string) {
	parsed, err := url.Parse(sessionURI)
	if err != nil {
		return
	}
	sessionID := strings.TrimPrefix(parsed.Path, "/fake-gcs/upload/")
	if sessionID == "" {
		return
	}
	f.mu.Lock()
	f.invalid[sessionID] = struct{}{}
	f.mu.Unlock()
}

type fakeResumableSigner struct {
	server *fakeGCSServer
	mu     sync.Mutex
	seq    int
}

func (f *fakeResumableSigner) SignedResumableInitURL(_ context.Context, bucket, objectName, contentType string, ttl time.Duration) (string, time.Time, error) {
	f.mu.Lock()
	f.seq++
	token := strconv.Itoa(f.seq)
	f.mu.Unlock()
	return f.server.SignedURL(bucket, objectName, contentType, token), time.Now().Add(ttl), nil
}

// --- 通用工具 ---

func boolPtr(v bool) *bool { return &v }

// --- Postgres + 迁移 ---

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
		t.Skipf("skip upload e2e: cannot start postgres container: %v", err)
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

func applyMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := filepath.Join("migrations")
	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	var paths []string
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".sql" {
			continue
		}
		paths = append(paths, filepath.Join(migrationsDir, file.Name()))
	}
	sort.Strings(paths)

	for _, path := range paths {
		sqlBytes, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		_, execErr := pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, execErr, "apply migration %s", filepath.Base(path))
	}
}
