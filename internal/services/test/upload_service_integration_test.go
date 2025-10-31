package services_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestUploadServiceIntegration_InitAndRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyAllMigrations(ctx, t, pool)

	repo := repositories.NewUploadRepository(pool, log.NewStdLogger(io.Discard))

	signer := &spySigner{
		url:     "https://signed.example/initial",
		expires: time.Now().Add(20 * time.Minute).UTC(),
	}
	svc, err := services.NewUploadService(repo, signer, "catalog-media", 15*time.Minute, log.NewStdLogger(io.Discard))
	require.NoError(t, err)

	userID := uuid.New()
	md5Hex := strings.Repeat("a", 32)

	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       4 * 1024 * 1024,
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 120,
		Title:           "Integration Upload",
		Description:     "Initial flow",
	}

	result, err := svc.InitResumableUpload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Reused)
	require.Equal(t, signer.url, result.ResumableInitURL)
	require.Equal(t, 1, signer.calls)

	session, err := repo.GetByVideoID(ctx, nil, result.Session.VideoID)
	require.NoError(t, err)
	require.Equal(t, "Integration Upload", session.Title)
	require.NotNil(t, session.SignedURL)
	require.Equal(t, signer.url, *session.SignedURL)

	second, err := svc.InitResumableUpload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.True(t, second.Reused)
	require.Equal(t, result.Session.VideoID, second.Session.VideoID)
	require.Equal(t, signer.url, second.ResumableInitURL)
	require.Equal(t, 1, signer.calls, "expected no additional signer invocation for reused session")

	_, err = pool.Exec(ctx, `
        update catalog.uploads
        set signed_url_expires_at = now() - interval '2 minutes'
        where video_id = $1
    `, result.Session.VideoID)
	require.NoError(t, err)

	signer.url = "https://signed.example/refreshed"
	signer.expires = time.Now().Add(45 * time.Minute).UTC()

	third, err := svc.InitResumableUpload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, third)
	require.True(t, third.Reused)
	require.Equal(t, result.Session.VideoID, third.Session.VideoID)
	require.Equal(t, signer.url, third.ResumableInitURL)
	require.Equal(t, 2, signer.calls, "expected signer to be called for refreshed session")

	afterRefresh, err := repo.GetByVideoID(ctx, nil, result.Session.VideoID)
	require.NoError(t, err)
	require.NotNil(t, afterRefresh.SignedURL)
	require.Equal(t, signer.url, *afterRefresh.SignedURL)
	require.NotNil(t, afterRefresh.SignedURLExpiresAt)
	require.WithinDuration(t, signer.expires, afterRefresh.SignedURLExpiresAt.UTC(), 2*time.Second)
}

func TestUploadServiceIntegration_ConcurrentInitSingleSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyAllMigrations(ctx, t, pool)

	repo := repositories.NewUploadRepository(pool, log.NewStdLogger(io.Discard))

	signer := &sequentialSigner{ttl: 15 * time.Minute}
	svc, err := services.NewUploadService(repo, signer, "catalog-media", 10*time.Minute, log.NewStdLogger(io.Discard))
	require.NoError(t, err)

	userID := uuid.New()
	md5Hex := strings.Repeat("b", 32)
	input := services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       8 * 1024 * 1024,
		ContentType:     "video/mp4",
		ContentMD5Hex:   md5Hex,
		DurationSeconds: 180,
		Title:           "Concurrent Upload",
		Description:     "Verify single session",
	}

	const workers = 5
	results := make([]*services.InitResumableUploadResult, workers)
	errs := make([]error, workers)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			res, err := svc.InitResumableUpload(ctx, input)
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

	var total int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from catalog.uploads`).Scan(&total))
	require.Equal(t, 1, total)

	finalSession, err := repo.GetByVideoID(ctx, nil, first.Session.VideoID)
	require.NoError(t, err)
	require.NotNil(t, finalSession.SignedURL)
}

type spySigner struct {
	url     string
	expires time.Time
	calls   int
}

func (s *spySigner) SignedResumableInitURL(_ context.Context, _ string, _ string, _ string, _ time.Duration) (string, time.Time, error) {
	s.calls++
	return s.url, s.expires, nil
}

type sequentialSigner struct {
	mu    sync.Mutex
	count int
	ttl   time.Duration
}

func (s *sequentialSigner) SignedResumableInitURL(_ context.Context, _ string, _ string, _ string, _ time.Duration) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	url := fmt.Sprintf("https://signed.example/%d", s.count)
	return url, time.Now().Add(s.ttl).UTC(), nil
}
