package repositories_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestVideoRepositoryIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	ensureAuthSchema(ctx, t, pool)
	applyMigrations(ctx, t, pool)

	repo := repositories.NewVideoRepository(pool, log.NewStdLogger(io.Discard))

	userA := uuid.New()
	userB := uuid.New()
	insertAuthUser(ctx, t, pool, userA, "user-a@example.com")
	insertAuthUser(ctx, t, pool, userB, "user-b@example.com")

	base := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)

	readyOlder := insertVideo(ctx, t, pool, videoSeed{
		VideoID:        uuid.New(),
		UploadUserID:   userA,
		Title:          "Ready Older",
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		CreatedAt:      base.Add(48 * time.Hour),
		Version:        2,
	})

	readyRecent := insertVideo(ctx, t, pool, videoSeed{
		VideoID:        uuid.New(),
		UploadUserID:   userA,
		Title:          "Ready Recent",
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		CreatedAt:      base.Add(72 * time.Hour),
		Version:        5,
	})

	publishedNewest := insertVideo(ctx, t, pool, videoSeed{
		VideoID:        uuid.New(),
		UploadUserID:   userB,
		Title:          "Published Newest",
		Status:         po.VideoStatusPublished,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		CreatedAt:      base.Add(96 * time.Hour),
		Version:        4,
	})

	processing := insertVideo(ctx, t, pool, videoSeed{
		VideoID:        uuid.New(),
		UploadUserID:   userA,
		Title:          "Processing",
		Status:         po.VideoStatusProcessing,
		MediaStatus:    po.StageProcessing,
		AnalysisStatus: po.StagePending,
		CreatedAt:      base.Add(36 * time.Hour),
		Version:        1,
	})

	failed := insertVideo(ctx, t, pool, videoSeed{
		VideoID:        uuid.New(),
		UploadUserID:   userA,
		Title:          "Failed",
		Status:         po.VideoStatusFailed,
		MediaStatus:    po.StageFailed,
		AnalysisStatus: po.StageFailed,
		CreatedAt:      base,
		Version:        3,
	})

	enrichMetadata(ctx, t, pool, readyRecent.VideoID)

	t.Run("ListPublicVideos", func(t *testing.T) {
		page1, err := repo.ListPublicVideos(ctx, nil, repositories.ListPublicVideosInput{Limit: 2})
		require.NoError(t, err)
		require.Len(t, page1, 2)
		require.Equal(t, publishedNewest.VideoID, page1[0].VideoID)
		require.Equal(t, readyRecent.VideoID, page1[1].VideoID)

		cursorCreated := page1[1].CreatedAt
		cursorID := page1[1].VideoID
		page2, err := repo.ListPublicVideos(ctx, nil, repositories.ListPublicVideosInput{
			Limit:           2,
			CursorCreatedAt: &cursorCreated,
			CursorVideoID:   &cursorID,
		})
		require.NoError(t, err)
		require.Len(t, page2, 1)
		require.Equal(t, readyOlder.VideoID, page2[0].VideoID)

		for _, item := range append(page1, page2...) {
			require.Contains(t, []po.VideoStatus{po.VideoStatusReady, po.VideoStatusPublished}, item.Status)
		}
	})

	t.Run("ListUserUploadsFiltersAndPagination", func(t *testing.T) {
		all, err := repo.ListUserUploads(ctx, nil, repositories.ListUserUploadsInput{
			UploadUserID: userA,
			Limit:        10,
		})
		require.NoError(t, err)
		require.Len(t, all, 4)
		require.Equal(t, readyRecent.VideoID, all[0].VideoID)
		require.Equal(t, readyOlder.VideoID, all[1].VideoID)
		require.Equal(t, processing.VideoID, all[2].VideoID)
		require.Equal(t, failed.VideoID, all[3].VideoID)
		require.EqualValues(t, 7, all[0].Version)

		readyOnly, err := repo.ListUserUploads(ctx, nil, repositories.ListUserUploadsInput{
			UploadUserID: userA,
			Limit:        10,
			StatusFilter: []po.VideoStatus{po.VideoStatusReady},
		})
		require.NoError(t, err)
		require.Len(t, readyOnly, 2)
		for _, item := range readyOnly {
			require.Equal(t, po.VideoStatusReady, item.Status)
		}

		stageFiltered, err := repo.ListUserUploads(ctx, nil, repositories.ListUserUploadsInput{
			UploadUserID: userA,
			Limit:        10,
			StageFilter:  []po.StageStatus{po.StageProcessing},
		})
		require.NoError(t, err)
		require.Len(t, stageFiltered, 1)
		require.Equal(t, processing.VideoID, stageFiltered[0].VideoID)

		page1, err := repo.ListUserUploads(ctx, nil, repositories.ListUserUploadsInput{
			UploadUserID: userA,
			Limit:        2,
		})
		require.NoError(t, err)
		require.Len(t, page1, 2)

		cursorCreated := page1[1].CreatedAt
		cursorID := page1[1].VideoID
		page2, err := repo.ListUserUploads(ctx, nil, repositories.ListUserUploadsInput{
			UploadUserID:    userA,
			Limit:           2,
			CursorCreatedAt: &cursorCreated,
			CursorVideoID:   &cursorID,
		})
		require.NoError(t, err)
		require.Len(t, page2, 2)
		require.Equal(t, processing.VideoID, page2[0].VideoID)
		require.Equal(t, failed.VideoID, page2[1].VideoID)
	})

	t.Run("GetMetadata", func(t *testing.T) {
		metadata, err := repo.GetMetadata(ctx, nil, readyRecent.VideoID)
		require.NoError(t, err)
		require.Equal(t, readyRecent.VideoID, metadata.VideoID)
		require.Equal(t, po.VideoStatusReady, metadata.Status)
		require.Equal(t, po.StageReady, metadata.MediaStatus)
		require.Equal(t, po.StageReady, metadata.AnalysisStatus)
		require.NotNil(t, metadata.DurationMicros)
		require.EqualValues(t, 180000000, *metadata.DurationMicros)
		require.NotNil(t, metadata.EncodedResolution)
		require.Equal(t, "1920x1080", *metadata.EncodedResolution)
		require.NotNil(t, metadata.EncodedBitrate)
		require.EqualValues(t, 4200, *metadata.EncodedBitrate)
		require.NotNil(t, metadata.ThumbnailURL)
		require.Equal(t, "https://cdn.example/ready-recent/thumbnail.jpg", *metadata.ThumbnailURL)
		require.NotNil(t, metadata.HLSMasterPlaylist)
		require.Equal(t, "https://cdn.example/ready-recent/master.m3u8", *metadata.HLSMasterPlaylist)
		require.NotNil(t, metadata.Difficulty)
		require.Equal(t, "intermediate", *metadata.Difficulty)
		require.NotNil(t, metadata.Summary)
		require.Equal(t, "Catalog integration metadata snapshot", *metadata.Summary)
		require.ElementsMatch(t, []string{"listening", "b2"}, metadata.Tags)
		require.NotNil(t, metadata.RawSubtitleURL)
		require.Equal(t, "gs://bucket/subtitles/ready-recent.vtt", *metadata.RawSubtitleURL)
		require.EqualValues(t, 7, metadata.Version)
	})
}

type videoSeed struct {
	VideoID        uuid.UUID
	UploadUserID   uuid.UUID
	Title          string
	Status         po.VideoStatus
	MediaStatus    po.StageStatus
	AnalysisStatus po.StageStatus
	CreatedAt      time.Time
	Version        int64
}

func insertVideo(ctx context.Context, t *testing.T, pool *pgxpool.Pool, seed videoSeed) videoSeed {
	t.Helper()

	_, err := pool.Exec(ctx, `
        INSERT INTO catalog.videos (
            video_id, upload_user_id, title, raw_file_reference,
            status, media_status, analysis_status,
            created_at, updated_at, version
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `,
		seed.VideoID,
		seed.UploadUserID,
		seed.Title,
		"gs://catalog/vod/"+seed.VideoID.String()+".mp4",
		seed.Status,
		seed.MediaStatus,
		seed.AnalysisStatus,
		seed.CreatedAt,
		seed.CreatedAt,
		seed.Version,
	)
	require.NoError(t, err)

	return seed
}

func enrichMetadata(ctx context.Context, t *testing.T, pool *pgxpool.Pool, videoID uuid.UUID) {
	t.Helper()

	duration := int64(180000000)
	bitrate := int32(4200)
	_, err := pool.Exec(ctx, `
        UPDATE catalog.videos
        SET duration_micros = $2,
            encoded_resolution = $3,
            encoded_bitrate = $4,
            thumbnail_url = $5,
            hls_master_playlist = $6,
            difficulty = $7,
            summary = $8,
            tags = $9,
            raw_subtitle_url = $10,
            version = $11
        WHERE video_id = $1
    `,
		videoID,
		duration,
		"1920x1080",
		bitrate,
		"https://cdn.example/ready-recent/thumbnail.jpg",
		"https://cdn.example/ready-recent/master.m3u8",
		"intermediate",
		"Catalog integration metadata snapshot",
		[]string{"listening", "b2"},
		"gs://bucket/subtitles/ready-recent.vtt",
		int64(7),
	)
	require.NoError(t, err)
}

func ensureAuthSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

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

func insertAuthUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, email string) {
	t.Helper()

	_, err := pool.Exec(ctx, `
        insert into auth.users (id, email)
        values ($1, $2)
        on conflict (id) do nothing
    `, userID, email)
	require.NoError(t, err)
}
