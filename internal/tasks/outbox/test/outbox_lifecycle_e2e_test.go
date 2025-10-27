package outbox_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/pstest"
	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/protobuf/proto"
)

var expectedLifecycleTypes = []videov1.EventType{
	videov1.EventType_EVENT_TYPE_VIDEO_CREATED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_AI_ENRICHED,
	videov1.EventType_EVENT_TYPE_VIDEO_UPDATED,
	videov1.EventType_EVENT_TYPE_VIDEO_VISIBILITY_CHANGED,
}

type lifecycleTestEnv struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	outboxRepo    *repositories.OutboxRepository
	registerSvc   *services.RegisterUploadService
	processingSvc *services.ProcessingStatusService
	mediaSvc      *services.MediaInfoService
	aiSvc         *services.AIAttributesService
	visibilitySvc *services.VisibilityService
	server        *pstest.Server
}

type lifecycleFlowResult struct {
	VideoID        uuid.UUID
	MediaJobID     string
	AnalysisJobID  string
	DurationMicros int64
	Resolution     string
	Bitrate        int32
	Thumbnail      string
	Playlist       string
	Difficulty     string
	Summary        string
	SubtitleURL    string
}

func TestOutboxPublisher_EndToEndLifecycle(t *testing.T) {
	env := newLifecycleTestEnv(t)
	result := runLifecycleFlow(t, env)

	msgs := waitForMessages(t, env.server, len(expectedLifecycleTypes))
	events := decodeMessages(t, msgs)
	require.Len(t, events, len(expectedLifecycleTypes))

	for i, evt := range events {
		require.Equal(t, expectedLifecycleTypes[i], evt.EventType)
		require.Equal(t, result.VideoID.String(), evt.AggregateId)
		require.Equal(t, "video", evt.AggregateType)

		switch evt.EventType {
		case videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY:
			payload := evt.GetMediaReady()
			require.NotNil(t, payload)
			require.Equal(t, result.MediaJobID, payload.GetJobId())
			require.Equal(t, "ready", payload.GetMediaStatus())
			require.Equal(t, result.Resolution, payload.GetEncodedResolution())
		case videov1.EventType_EVENT_TYPE_VIDEO_AI_ENRICHED:
			payload := evt.GetAiEnriched()
			require.NotNil(t, payload)
			require.Equal(t, result.AnalysisJobID, payload.GetJobId())
			require.Equal(t, result.Difficulty, payload.GetDifficulty())
			require.Equal(t, result.Summary, payload.GetSummary())
			require.Equal(t, result.SubtitleURL, payload.GetRawSubtitleUrl())
		case videov1.EventType_EVENT_TYPE_VIDEO_VISIBILITY_CHANGED:
			payload := evt.GetVisibilityChanged()
			require.NotNil(t, payload)
			require.Equal(t, "published", payload.GetStatus())
			require.Equal(t, "ready", payload.GetPreviousStatus())
		}
	}

	pending, err := env.outboxRepo.CountPending(env.ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), pending)
}

func TestOutboxPublisher_MediaReadyIdempotent(t *testing.T) {
	env := newLifecycleTestEnv(t)
	result := runLifecycleFlow(t, env)

	initialMsgs := waitForMessages(t, env.server, len(expectedLifecycleTypes))
	initialEvents := decodeMessages(t, initialMsgs)
	require.Equal(t, 1, countEventType(initialEvents, videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY))

	mediaStatus := po.StageReady
	_, err := env.mediaSvc.UpdateMediaInfo(env.ctx, services.UpdateMediaInfoInput{
		VideoID:           result.VideoID,
		DurationMicros:    &result.DurationMicros,
		EncodedResolution: &result.Resolution,
		EncodedBitrate:    &result.Bitrate,
		ThumbnailURL:      &result.Thumbnail,
		HLSMasterPlaylist: &result.Playlist,
		MediaStatus:       &mediaStatus,
		JobID:             result.MediaJobID,
		EmittedAt:         time.Now().UTC(),
	})
	require.NoError(t, err)

	updatedMsgs := waitForMessages(t, env.server, len(initialMsgs)+1)
	updatedEvents := decodeMessages(t, updatedMsgs)
	require.Equal(t, len(initialEvents)+1, len(updatedEvents))
	require.Equal(t, 1, countEventType(updatedEvents, videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY))
	last := updatedEvents[len(updatedEvents)-1]
	require.Equal(t, videov1.EventType_EVENT_TYPE_VIDEO_UPDATED, last.EventType)

	pending, err := env.outboxRepo.CountPending(env.ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), pending)
}

func newLifecycleTestEnv(t *testing.T) *lifecycleTestEnv {
	t.Helper()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	t.Cleanup(terminate)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	ensureAuthUsersTable(ctx, t, pool)
	applyMigrations(ctx, t, pool)

	logger := log.NewStdLogger(io.Discard)
	outboxRepo := repositories.NewOutboxRepository(pool, logger, defaultOutboxConfig)
	videoRepo := repositories.NewVideoRepository(pool, logger)
	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: logger})
	require.NoError(t, err)

	writer := services.NewLifecycleWriter(videoRepo, outboxRepo, txMgr, logger)
	env := &lifecycleTestEnv{
		ctx:           ctx,
		pool:          pool,
		outboxRepo:    outboxRepo,
		registerSvc:   services.NewRegisterUploadService(writer),
		processingSvc: services.NewProcessingStatusService(writer, videoRepo),
		mediaSvc:      services.NewMediaInfoService(writer, videoRepo),
		aiSvc:         services.NewAIAttributesService(writer, videoRepo),
		visibilitySvc: services.NewVisibilityService(writer, videoRepo),
	}

	server := pstest.NewServer()
	t.Cleanup(func() { _ = server.Close() })
	env.server = server

	component, cleanupPublisher, publisher := newTestPublisher(ctx, t, server, "catalog-test", "catalog-video-events")
	t.Cleanup(cleanupPublisher)
	t.Cleanup(func() { _ = component })

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(ctx) })
	meter := meterProvider.Meter("lingo-services-catalog.outbox.e2e")

	runner := newPublisherRunner(t, outboxRepo, publisher, meter, outboxcfg.PublisherConfig{
		BatchSize:      1,
		TickInterval:   25 * time.Millisecond,
		InitialBackoff: 25 * time.Millisecond,
		MaxBackoff:     250 * time.Millisecond,
		MaxAttempts:    3,
		PublishTimeout: time.Second,
		Workers:        1,
		LockTTL:        time.Second,
	})

	runCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("outbox runner error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("outbox runner did not stop in time")
		}
	})

	return env
}

func runLifecycleFlow(t *testing.T, env *lifecycleTestEnv) lifecycleFlowResult {
	t.Helper()

	ctx := env.ctx
	uploaderID := uuid.New()
	insertUser(ctx, t, env.pool, uploaderID)

	created, err := env.registerSvc.RegisterUpload(ctx, services.RegisterUploadInput{
		UploadUserID:     uploaderID,
		Title:            "Lifecycle E2E",
		Description:      strPtr("integration test flow"),
		RawFileReference: "gs://learning-app/raw/video.mp4",
	})
	require.NoError(t, err)
	videoID := created.VideoID

	result := lifecycleFlowResult{
		VideoID:        videoID,
		MediaJobID:     "media-job-001",
		AnalysisJobID:  "analysis-job-001",
		DurationMicros: 120_000_000,
		Resolution:     "1920x1080",
		Bitrate:        3200,
		Thumbnail:      "https://cdn.example/thumb.jpg",
		Playlist:       "https://cdn.example/master.m3u8",
		Difficulty:     "B2",
		Summary:        "Test summary for AI enrichment",
		SubtitleURL:    "https://cdn.example/subtitle.vtt",
	}

	mediaStart := time.Now().UTC().Add(50 * time.Millisecond)
	require.NoError(t, invokeProcessing(env.processingSvc, services.ProcessingStageMedia, videoID, po.StagePending, po.StageProcessing, result.MediaJobID, mediaStart, nil))
	mediaReadyAt := mediaStart.Add(150 * time.Millisecond)
	require.NoError(t, invokeProcessing(env.processingSvc, services.ProcessingStageMedia, videoID, po.StageProcessing, po.StageReady, result.MediaJobID, mediaReadyAt, nil))

	mediaStatus := po.StageReady
	_, err = env.mediaSvc.UpdateMediaInfo(ctx, services.UpdateMediaInfoInput{
		VideoID:           videoID,
		DurationMicros:    &result.DurationMicros,
		EncodedResolution: &result.Resolution,
		EncodedBitrate:    &result.Bitrate,
		ThumbnailURL:      &result.Thumbnail,
		HLSMasterPlaylist: &result.Playlist,
		MediaStatus:       &mediaStatus,
		JobID:             result.MediaJobID,
		EmittedAt:         mediaReadyAt,
	})
	require.NoError(t, err)

	analysisStart := mediaReadyAt.Add(100 * time.Millisecond)
	require.NoError(t, invokeProcessing(env.processingSvc, services.ProcessingStageAnalysis, videoID, po.StagePending, po.StageProcessing, result.AnalysisJobID, analysisStart, nil))
	analysisReadyAt := analysisStart.Add(150 * time.Millisecond)
	require.NoError(t, invokeProcessing(env.processingSvc, services.ProcessingStageAnalysis, videoID, po.StageProcessing, po.StageReady, result.AnalysisJobID, analysisReadyAt, nil))

	analysisStatus := po.StageReady
	_, err = env.aiSvc.UpdateAIAttributes(ctx, services.UpdateAIAttributesInput{
		VideoID:        videoID,
		Difficulty:     &result.Difficulty,
		Summary:        &result.Summary,
		Tags:           []string{"integration", "demo"},
		RawSubtitleURL: &result.SubtitleURL,
		AnalysisStatus: &analysisStatus,
		JobID:          result.AnalysisJobID,
		EmittedAt:      analysisReadyAt,
	})
	require.NoError(t, err)

	_, err = env.visibilitySvc.UpdateVisibility(ctx, services.UpdateVisibilityInput{
		VideoID: videoID,
		Action:  services.VisibilityPublish,
	})
	require.NoError(t, err)

	return result
}

func waitForMessages(t *testing.T, server *pstest.Server, want int) []*pstest.Message {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(server.Messages()) >= want
	}, 10*time.Second, 50*time.Millisecond, "pubsub did not receive enough messages")
	return server.Messages()
}

func decodeMessages(t *testing.T, msgs []*pstest.Message) []*videov1.Event {
	t.Helper()
	events := make([]*videov1.Event, len(msgs))
	for i, msg := range msgs {
		var evt videov1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &evt))
		events[i] = &evt
	}
	return events
}

func countEventType(events []*videov1.Event, typ videov1.EventType) int {
	count := 0
	for _, evt := range events {
		if evt.EventType == typ {
			count++
		}
	}
	return count
}

func ensureAuthUsersTable(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, "create schema if not exists auth")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		create table if not exists auth.users (
			id uuid primary key,
			email text,
			created_at timestamptz default now()
		)
	`)
	require.NoError(t, err)
}

func insertUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	_, err := pool.Exec(ctx, "insert into auth.users (id, email) values ($1, $2) on conflict (id) do nothing", userID, "tester@example.com")
	require.NoError(t, err)
}

func invokeProcessing(svc *services.ProcessingStatusService, stage services.ProcessingStage, videoID uuid.UUID, expected po.StageStatus, next po.StageStatus, jobID string, emittedAt time.Time, errMsg *string) error {
	_, err := svc.UpdateProcessingStatus(context.Background(), services.UpdateProcessingStatusInput{
		VideoID:        videoID,
		Stage:          stage,
		ExpectedStatus: stagePtr(expected),
		NewStatus:      next,
		JobID:          jobID,
		EmittedAt:      emittedAt,
		ErrorMessage:   errMsg,
	})
	return err
}

func stagePtr(status po.StageStatus) *po.StageStatus {
	value := status
	return &value
}

func strPtr(v string) *string {
	value := v
	return &value
}
