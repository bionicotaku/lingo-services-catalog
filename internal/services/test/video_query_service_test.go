package services_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

func TestVideoQueryService_ListMyUploadsRequiresUserID(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	_, _, err := svc.ListMyUploads(context.Background(), 10, "", nil, nil)
	if err == nil {
		t.Fatalf("expected error when user metadata missing")
	}
	e := errors.FromError(err)
	if e.Code != 401 {
		t.Fatalf("expected http 401, got %d (%s)", e.Code, e.Message)
	}
}

func TestVideoQueryService_ListMyUploadsInvalidUserID(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	ctx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: "not-a-uuid"})

	_, _, err := svc.ListMyUploads(ctx, 10, "", nil, nil)
	if err == nil {
		t.Fatalf("expected error for invalid user id")
	}
	e := errors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected http 400, got %d (%s)", e.Code, e.Message)
	}
}

func TestVideoQueryService_ListMyUploadsInvalidUserInfo(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	ctx := metadata.Inject(context.Background(), metadata.HandlerMetadata{RawUserInfo: "broken", InvalidUserInfo: true})

	_, _, err := svc.ListMyUploads(ctx, 10, "", nil, nil)
	if err == nil {
		t.Fatalf("expected error for invalid user info")
	}
	e := errors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected http 400, got %d (%s)", e.Code, e.Message)
	}
}

func TestVideoQueryService_GetVideoDetailInvalidUserID(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	ctx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: "invalid"})

	_, _, err := svc.GetVideoDetail(ctx, uuid.New())
	if err == nil {
		t.Fatalf("expected error for invalid user id metadata")
	}
	e := errors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected http 400, got %d (%s)", e.Code, e.Message)
	}
}

func TestVideoQueryService_GetVideoDetailInvalidUserInfo(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	ctx := metadata.Inject(context.Background(), metadata.HandlerMetadata{RawUserInfo: "broken", InvalidUserInfo: true})

	_, _, err := svc.GetVideoDetail(ctx, uuid.New())
	if err == nil {
		t.Fatalf("expected error for invalid user info metadata")
	}
	e := errors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected http 400, got %d (%s)", e.Code, e.Message)
	}
}

func TestVideoQueryService_GetVideoMetadataSuccess(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoID := uuid.New()
	now := time.Now().UTC()
	repo := &queryRepoStub{
		metadata: &po.VideoMetadata{
			VideoID:           videoID,
			Status:            po.VideoStatusReady,
			MediaStatus:       po.StageReady,
			AnalysisStatus:    po.StageProcessing,
			DurationMicros:    ptrInt64(120_000_000),
			EncodedResolution: ptrString("1920x1080"),
			EncodedBitrate:    ptrInt32(4200),
			ThumbnailURL:      ptrString("https://example/thumb.jpg"),
			HLSMasterPlaylist: ptrString("https://example/master.m3u8"),
			Difficulty:        ptrString("B2"),
			Summary:           ptrString("demo"),
			Tags:              []string{"tag1", "tag2"},
			RawSubtitleURL:    ptrString("https://example/sub.vtt"),
			UpdatedAt:         now,
			Version:           3,
		},
	}
	svc := services.NewVideoQueryService(repo, nil, noopTxManager{}, logger)

	meta, err := svc.GetVideoMetadata(context.Background(), videoID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Version != 3 || meta.EncodedResolution != "1920x1080" {
		t.Fatalf("metadata not propagated: %+v", meta)
	}
}

func TestVideoQueryService_GetVideoDetailSuccess(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoID := uuid.New()
	now := time.Now().UTC()
	repo := &queryRepoStub{
		detail: &po.VideoReadyView{
			VideoID:        videoID,
			Title:          "Demo",
			Status:         po.VideoStatusReady,
			MediaStatus:    po.StageReady,
			AnalysisStatus: po.StageProcessing,
			CreatedAt:      now.Add(-time.Hour),
			UpdatedAt:      now,
		},
		metadata: &po.VideoMetadata{
			VideoID:        videoID,
			Status:         po.VideoStatusReady,
			MediaStatus:    po.StageReady,
			AnalysisStatus: po.StageProcessing,
			UpdatedAt:      now,
			Version:        5,
		},
	}
	svc := services.NewVideoQueryService(repo, nil, noopTxManager{}, logger)

	ctx := context.Background()
	detail, meta, err := svc.GetVideoDetail(ctx, videoID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatalf("detail should not be nil")
	}
	if meta == nil || meta.Version != 5 {
		t.Fatalf("metadata missing: %+v", meta)
	}
	if repo.detailCalls != 1 || repo.metadataCalls != 1 {
		t.Fatalf("expected repo invocation counts (detail=%d metadata=%d)", repo.detailCalls, repo.metadataCalls)
	}
}

func TestVideoQueryService_ListMyUploadsAppliesFilters(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	userID := uuid.New()
	repo := &queryRepoStub{
		uploads: []po.MyUploadEntry{
			{
				VideoID:        uuid.New(),
				Title:          "Video",
				Status:         po.VideoStatusProcessing,
				MediaStatus:    po.StageProcessing,
				AnalysisStatus: po.StagePending,
				Version:        1,
				CreatedAt:      time.Now().UTC(),
				UpdatedAt:      time.Now().UTC(),
			},
		},
	}
	svc := services.NewVideoQueryService(repo, nil, noopTxManager{}, logger)

	ctx := metadata.Inject(context.Background(), metadata.HandlerMetadata{UserID: userID.String()})
	statusFilters := []po.VideoStatus{po.VideoStatusProcessing}
	stageFilters := []po.StageStatus{po.StageProcessing}

	items, token, err := svc.ListMyUploads(ctx, 10, "", statusFilters, stageFilters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Fatalf("expected empty next token")
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if got := repo.lastUploadsInput.StatusFilter; len(got) != 1 || got[0] != po.VideoStatusProcessing {
		t.Fatalf("status filter not forwarded: %+v", got)
	}
	if got := repo.lastUploadsInput.StageFilter; len(got) != 1 || got[0] != po.StageProcessing {
		t.Fatalf("stage filter not forwarded: %+v", got)
	}
}

type queryRepoStub struct {
	detail           *po.VideoReadyView
	metadata         *po.VideoMetadata
	uploads          []po.MyUploadEntry
	detailCalls      int
	metadataCalls    int
	lastUploadsInput repositories.ListUserUploadsInput
}

func (q *queryRepoStub) FindPublishedByID(context.Context, txmanager.Session, uuid.UUID) (*po.VideoReadyView, error) {
	q.detailCalls++
	if q.detail == nil {
		return nil, repositories.ErrVideoNotFound
	}
	return q.detail, nil
}

func (q *queryRepoStub) GetMetadata(context.Context, txmanager.Session, uuid.UUID) (*po.VideoMetadata, error) {
	q.metadataCalls++
	if q.metadata == nil {
		return nil, repositories.ErrVideoNotFound
	}
	return q.metadata, nil
}

func (q *queryRepoStub) ListPublicVideos(context.Context, txmanager.Session, repositories.ListPublicVideosInput) ([]po.VideoListEntry, error) {
	return nil, nil
}

func (q *queryRepoStub) ListUserUploads(_ context.Context, _ txmanager.Session, input repositories.ListUserUploadsInput) ([]po.MyUploadEntry, error) {
	q.lastUploadsInput = input
	return q.uploads, nil
}

func ptrString(v string) *string { return &v }

func ptrInt32(v int32) *int32 { return &v }

func ptrInt64(v int64) *int64 { return &v }
