package repositories_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestUploadRepository_UpsertAndQueries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewUploadRepository(pool, log.NewStdLogger(io.Discard))

	userID := uuid.New()
	videoID := uuid.New()
	objectName := fmt.Sprintf("raw_videos/%s/%s", userID, videoID)
	contentType := "video/mp4"
	contentMD5 := strings.Repeat("a", 32)
	signedURL := "https://signed.example/init"
	expires := time.Now().Add(20 * time.Minute).UTC()

	session, inserted, err := repo.Upsert(ctx, nil, repositories.UpsertUploadInput{
		VideoID:            videoID,
		UserID:             userID,
		Bucket:             "catalog-media",
		ObjectName:         objectName,
		ContentType:        &contentType,
		ExpectedSize:       4 * 1024 * 1024,
		ContentMD5:         contentMD5,
		Title:              "First Title",
		Description:        "First Description",
		SignedURL:          &signedURL,
		SignedURLExpiresAt: &expires,
	})
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, videoID, session.VideoID)
	require.Equal(t, po.UploadStatusUploading, session.Status)
	require.NotNil(t, session.SignedURL)
	require.Equal(t, signedURL, *session.SignedURL)
	require.NotNil(t, session.SignedURLExpiresAt)

	foundByVideo, err := repo.GetByVideoID(ctx, nil, videoID)
	require.NoError(t, err)
	require.Equal(t, session.VideoID, foundByVideo.VideoID)
	require.Equal(t, contentMD5, foundByVideo.ContentMD5)

	foundByObject, err := repo.GetByObject(ctx, nil, "catalog-media", objectName)
	require.NoError(t, err)
	require.Equal(t, session.VideoID, foundByObject.VideoID)

	foundByUserMD5, err := repo.GetByUserMD5(ctx, nil, userID, contentMD5)
	require.NoError(t, err)
	require.Equal(t, session.VideoID, foundByUserMD5.VideoID)

	updateURL := "https://signed.example/refresh"
	updateExpires := time.Now().Add(45 * time.Minute).UTC()
	updatedTitle := "Updated Title"
	updatedDesc := "Updated Description"

	sessionAfterUpdate, insertedAfterUpdate, err := repo.Upsert(ctx, nil, repositories.UpsertUploadInput{
		VideoID:            videoID,
		UserID:             userID,
		Bucket:             "catalog-media",
		ObjectName:         objectName,
		ContentType:        &contentType,
		ExpectedSize:       6 * 1024 * 1024,
		ContentMD5:         contentMD5,
		Title:              updatedTitle,
		Description:        updatedDesc,
		SignedURL:          &updateURL,
		SignedURLExpiresAt: &updateExpires,
	})
	require.NoError(t, err)
	require.False(t, insertedAfterUpdate)
	require.Equal(t, videoID, sessionAfterUpdate.VideoID)
	require.Equal(t, updatedTitle, sessionAfterUpdate.Title)
	require.Equal(t, updatedDesc, sessionAfterUpdate.Description)
	require.NotNil(t, sessionAfterUpdate.SignedURL)
	require.Equal(t, updateURL, *sessionAfterUpdate.SignedURL)

	expiredVideo := uuid.New()
	expiredMD5 := strings.Repeat("b", 32)
	expiredObject := fmt.Sprintf("raw_videos/%s/%s", userID, expiredVideo)
	expiredURL := "https://signed.example/expired"
	expiredAt := time.Now().Add(-10 * time.Minute).UTC()

	_, insertedExpired, err := repo.Upsert(ctx, nil, repositories.UpsertUploadInput{
		VideoID:            expiredVideo,
		UserID:             userID,
		Bucket:             "catalog-media",
		ObjectName:         expiredObject,
		ContentType:        &contentType,
		ExpectedSize:       1234,
		ContentMD5:         expiredMD5,
		Title:              "Expired Upload",
		Description:        "Expired Signed URL",
		SignedURL:          &expiredURL,
		SignedURLExpiresAt: &expiredAt,
	})
	require.NoError(t, err)
	require.True(t, insertedExpired)

	futureVideo := uuid.New()
	futureMD5 := strings.Repeat("c", 32)
	futureObject := fmt.Sprintf("raw_videos/%s/%s", userID, futureVideo)
	futureURL := "https://signed.example/future"
	futureAt := time.Now().Add(30 * time.Minute).UTC()

	_, insertedFuture, err := repo.Upsert(ctx, nil, repositories.UpsertUploadInput{
		VideoID:            futureVideo,
		UserID:             userID,
		Bucket:             "catalog-media",
		ObjectName:         futureObject,
		ContentType:        &contentType,
		ExpectedSize:       4321,
		ContentMD5:         futureMD5,
		Title:              "Future Upload",
		Description:        "Still Valid",
		SignedURL:          &futureURL,
		SignedURLExpiresAt: &futureAt,
	})
	require.NoError(t, err)
	require.True(t, insertedFuture)

	expiredSessions, err := repo.ListExpiredUploads(ctx, nil, time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Len(t, expiredSessions, 1)
	require.Equal(t, expiredVideo, expiredSessions[0].VideoID)
	require.Equal(t, po.UploadStatusUploading, expiredSessions[0].Status)
}

func TestUploadRepository_MarkCompletedAndFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewUploadRepository(pool, log.NewStdLogger(io.Discard))

	userID := uuid.New()
	videoID := uuid.New()
	contentType := "video/mp4"
	contentMD5 := strings.Repeat("d", 32)
	objectName := fmt.Sprintf("raw_videos/%s/%s", userID, videoID)
	signedURL := "https://signed.example/completed"
	expires := time.Now().Add(10 * time.Minute).UTC()

	_, inserted, err := repo.Upsert(ctx, nil, repositories.UpsertUploadInput{
		VideoID:            videoID,
		UserID:             userID,
		Bucket:             "catalog-media",
		ObjectName:         objectName,
		ContentType:        &contentType,
		ExpectedSize:       1_000_000,
		ContentMD5:         contentMD5,
		Title:              "Complete Me",
		Description:        "Ready to complete",
		SignedURL:          &signedURL,
		SignedURLExpiresAt: &expires,
	})
	require.NoError(t, err)
	require.True(t, inserted)

	md5Hash := strings.Repeat("ab", 16)
	crc32c := "AAAAAA=="
	generation := "123"
	etag := "etag-1"
	completed, err := repo.MarkCompleted(ctx, nil, repositories.MarkUploadCompletedInput{
		VideoID:       videoID,
		SizeBytes:     1_048_576,
		MD5Hash:       &md5Hash,
		CRC32C:        &crc32c,
		GCSGeneration: &generation,
		GCSEtag:       &etag,
		ContentType:   &contentType,
	})
	require.NoError(t, err)
	require.Equal(t, po.UploadStatusCompleted, completed.Status)
	require.Nil(t, completed.SignedURL)
	require.Nil(t, completed.SignedURLExpiresAt)
	require.Equal(t, int64(1_048_576), completed.SizeBytes)
	require.NotNil(t, completed.MD5Hash)
	require.Equal(t, strings.ToLower(md5Hash), *completed.MD5Hash)
	require.NotNil(t, completed.GCSGeneration)
	require.Equal(t, generation, *completed.GCSGeneration)

	errorCode := "MD5_MISMATCH"
	errorMessage := "hash mismatch"
	failed, err := repo.MarkFailed(ctx, nil, repositories.MarkUploadFailedInput{
		VideoID:      videoID,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})
	require.NoError(t, err)
	require.Equal(t, po.UploadStatusFailed, failed.Status)
	require.NotNil(t, failed.ErrorCode)
	require.Equal(t, errorCode, *failed.ErrorCode)
	require.NotNil(t, failed.ErrorMessage)
	require.Equal(t, errorMessage, *failed.ErrorMessage)
}
