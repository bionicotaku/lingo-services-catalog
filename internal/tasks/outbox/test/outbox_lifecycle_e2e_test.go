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
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/protobuf/proto"

	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
)

// TestOutboxPublisher_EndToEndLifecycle 验证 Catalog 写模型通过 Outbox → Pub/Sub 的完整事件链路。
func TestOutboxPublisher_EndToEndLifecycle(t *testing.T) {
	ctx := context.Background()

	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	ensureAuthUsersTable(ctx, t, pool)
	applyMigrations(ctx, t, pool)

	repoLogger := log.NewStdLogger(io.Discard)
	outboxRepo := repositories.NewOutboxRepository(pool, repoLogger, defaultOutboxConfig)
	videoRepo := repositories.NewVideoRepository(pool, repoLogger)

	txMgr, err := txmanager.NewManager(pool, txmanager.Config{}, txmanager.Dependencies{Logger: repoLogger})
	require.NoError(t, err)

	commandSvc := services.NewVideoCommandService(videoRepo, outboxRepo, txMgr, repoLogger)
	registerSvc := services.NewRegisterUploadService(commandSvc)
	processingSvc := services.NewProcessingStatusService(commandSvc, videoRepo)
	mediaSvc := services.NewMediaInfoService(commandSvc, videoRepo)
	aiSvc := services.NewAIAttributesService(commandSvc, videoRepo)
	visibilitySvc := services.NewVisibilityService(commandSvc, videoRepo)

	pstestServer := pstest.NewServer()
	t.Cleanup(func() { _ = pstestServer.Close() })

	projectID := "catalog-test"
	topicID := "catalog-video-events"
	component, cleanupPublisher, publisher := newTestPublisher(ctx, t, pstestServer, projectID, topicID)
	defer cleanupPublisher()
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

	runCtx, cancelRun := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(runCtx) }()
	defer func() {
		cancelRun()
		select {
		case runErr := <-errCh:
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				require.NoError(t, runErr)
			}
		case <-time.After(time.Second):
			t.Fatalf("outbox runner did not stop in time")
		}
	}()

	uploaderID := uuid.New()
	insertUser(ctx, t, pool, uploaderID)

	created, err := registerSvc.RegisterUpload(ctx, services.RegisterUploadInput{
		UploadUserID:     uploaderID,
		Title:            "Lifecycle E2E",
		Description:      strPtr("integration test flow"),
		RawFileReference: "gs://learning-app/raw/video.mp4",
	})
	require.NoError(t, err)
	videoID := created.VideoID

	mediaJobID := "media-job-001"
	mediaStart := time.Now().UTC().Add(50 * time.Millisecond)
	require.NoError(t, invokeProcessing(processingSvc, services.ProcessingStageMedia, videoID, po.StagePending, po.StageProcessing, mediaJobID, mediaStart, nil))

	mediaReadyAt := mediaStart.Add(150 * time.Millisecond)
	require.NoError(t, invokeProcessing(processingSvc, services.ProcessingStageMedia, videoID, po.StageProcessing, po.StageReady, mediaJobID, mediaReadyAt, nil))

	duration := int64(120_000_000)
	resolution := "1920x1080"
	bitrate := int32(3200)
	thumbnail := "https://cdn.example/thumb.jpg"
	playlist := "https://cdn.example/master.m3u8"
	mediaStatus := po.StageReady
	_, err = mediaSvc.UpdateMediaInfo(ctx, services.UpdateMediaInfoInput{
		VideoID:           videoID,
		DurationMicros:    &duration,
		EncodedResolution: &resolution,
		EncodedBitrate:    &bitrate,
		ThumbnailURL:      &thumbnail,
		HLSMasterPlaylist: &playlist,
		MediaStatus:       &mediaStatus,
	})
	require.NoError(t, err)

	analysisJobID := "analysis-job-001"
	analysisStart := mediaReadyAt.Add(100 * time.Millisecond)
	require.NoError(t, invokeProcessing(processingSvc, services.ProcessingStageAnalysis, videoID, po.StagePending, po.StageProcessing, analysisJobID, analysisStart, nil))

	analysisReadyAt := analysisStart.Add(150 * time.Millisecond)
	require.NoError(t, invokeProcessing(processingSvc, services.ProcessingStageAnalysis, videoID, po.StageProcessing, po.StageReady, analysisJobID, analysisReadyAt, nil))

	difficulty := "B2"
	summary := "Test summary for AI enrichment"
	subtitleURL := "https://cdn.example/subtitle.vtt"
	analysisStatus := po.StageReady
	_, err = aiSvc.UpdateAIAttributes(ctx, services.UpdateAIAttributesInput{
		VideoID:        videoID,
		Difficulty:     &difficulty,
		Summary:        &summary,
		RawSubtitleURL: &subtitleURL,
		AnalysisStatus: &analysisStatus,
	})
	require.NoError(t, err)

	_, err = visibilitySvc.UpdateVisibility(ctx, services.UpdateVisibilityInput{
		VideoID: videoID,
		Action:  services.VisibilityPublish,
	})
	require.NoError(t, err)

	expectedTypes := []videov1.EventType{
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

	require.Eventually(t, func() bool {
		return len(pstestServer.Messages()) >= len(expectedTypes)
	}, 10*time.Second, 50*time.Millisecond, "pubsub did not receive all events")

	msgs := pstestServer.Messages()
	require.Len(t, msgs, len(expectedTypes))

	for i, msg := range msgs {
		var evt videov1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &evt))
		require.Equal(t, expectedTypes[i], evt.EventType)
		require.Equal(t, videoID.String(), evt.AggregateId)
		require.Equal(t, "video", evt.AggregateType)

		switch evt.EventType {
		case videov1.EventType_EVENT_TYPE_VIDEO_MEDIA_READY:
			payload := evt.GetMediaReady()
			require.NotNil(t, payload)
			require.Equal(t, mediaJobID, payload.GetJobId())
			require.Equal(t, "ready", payload.GetMediaStatus())
			require.Equal(t, resolution, payload.GetEncodedResolution())
		case videov1.EventType_EVENT_TYPE_VIDEO_AI_ENRICHED:
			payload := evt.GetAiEnriched()
			require.NotNil(t, payload)
			require.Equal(t, analysisJobID, payload.GetJobId())
			require.Equal(t, difficulty, payload.GetDifficulty())
			require.Equal(t, summary, payload.GetSummary())
			foundSubtitle := payload.GetRawSubtitleUrl()
			require.Equal(t, subtitleURL, foundSubtitle)
		case videov1.EventType_EVENT_TYPE_VIDEO_VISIBILITY_CHANGED:
			payload := evt.GetVisibilityChanged()
			require.NotNil(t, payload)
			require.Equal(t, "published", payload.GetStatus())
			require.Equal(t, "ready", payload.GetPreviousStatus())
		}
	}

	pending, err := outboxRepo.CountPending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), pending, "outbox should have no pending events")
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
