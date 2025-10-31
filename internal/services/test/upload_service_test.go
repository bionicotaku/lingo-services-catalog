package services_test

import (
	"context"
	"strings"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/bionicotaku/lingo-utils/txmanager"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

type stubSigner struct {
	url     string
	expires time.Time
	err     error
}

func (s stubSigner) SignedResumableInitURL(_ context.Context, _ string, _ string, _ string, _ time.Duration) (string, time.Time, error) {
	return s.url, s.expires, s.err
}

type stubUploadRepo struct {
	existing *po.UploadSession
	session  *po.UploadSession
	inserted bool
	upsert   repositories.UpsertUploadInput
}

func (s *stubUploadRepo) Upsert(_ context.Context, _ txmanager.Session, input repositories.UpsertUploadInput) (*po.UploadSession, bool, error) {
	s.upsert = input
	if s.session != nil {
		return s.session, s.inserted, nil
	}
	sess := &po.UploadSession{
		VideoID:            input.VideoID,
		UserID:             input.UserID,
		Bucket:             input.Bucket,
		ObjectName:         input.ObjectName,
		SignedURL:          input.SignedURL,
		SignedURLExpiresAt: input.SignedURLExpiresAt,
		Status:             po.UploadStatusUploading,
	}
	return sess, true, nil
}

func (s *stubUploadRepo) GetByUserMD5(_ context.Context, _ txmanager.Session, _ uuid.UUID, _ string) (*po.UploadSession, error) {
	if s.existing == nil {
		return nil, repositories.ErrUploadNotFound
	}
	return s.existing, nil
}

func TestUploadService_FirstSession(t *testing.T) {
	repo := &stubUploadRepo{}
	signer := stubSigner{url: "https://signed.example", expires: time.Now().Add(10 * time.Minute)}
	svc := newUploadService(t, repo, signer)

	result, err := svc.InitResumableUpload(context.Background(), services.InitResumableUploadInput{
		UserID:          uuid.New(),
		ContentType:     "video/mp4",
		ContentMD5Hex:   strings.Repeat("a", 32),
		Title:           "Title",
		Description:     "Description",
		DurationSeconds: 60,
		SizeBytes:       1024,
	})
	if err != nil {
		t.Fatalf("InitResumableUpload: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected fresh session")
	}
	if result.ResumableInitURL != signer.url {
		t.Fatalf("unexpected url: %s", result.ResumableInitURL)
	}
	if repo.upsert.VideoID == uuid.Nil {
		t.Fatalf("expected generated video id")
	}
}

func TestUploadService_ReusesExistingSignedURL(t *testing.T) {
	userID := uuid.New()
	existing := &po.UploadSession{
		VideoID:            uuid.New(),
		UserID:             userID,
		Bucket:             "bucket",
		ObjectName:         "raw_videos/user/video",
		SignedURL:          ptr("https://existing"),
		SignedURLExpiresAt: ptrTime(time.Now().Add(5 * time.Minute)),
		Status:             po.UploadStatusUploading,
	}
	repo := &stubUploadRepo{existing: existing, session: existing, inserted: false}
	signer := stubSigner{url: "https://new", expires: time.Now().Add(10 * time.Minute)}
	svc := newUploadService(t, repo, signer)

	result, err := svc.InitResumableUpload(context.Background(), services.InitResumableUploadInput{
		UserID:          userID,
		ContentType:     "video/mp4",
		ContentMD5Hex:   strings.Repeat("b", 32),
		Title:           "Title",
		Description:     "Description",
		DurationSeconds: 30,
	})
	if err != nil {
		t.Fatalf("InitResumableUpload: %v", err)
	}
	if !result.Reused {
		t.Fatalf("expected reused session")
	}
	if result.ResumableInitURL != "https://existing" {
		t.Fatalf("expected existing url, got %s", result.ResumableInitURL)
	}
}

func TestUploadService_CompletedReturnsConflict(t *testing.T) {
	userID := uuid.New()
	repo := &stubUploadRepo{existing: &po.UploadSession{VideoID: uuid.New(), UserID: userID, Status: po.UploadStatusCompleted}}
	signer := stubSigner{url: "https://signed", expires: time.Now().Add(time.Minute)}
	svc := newUploadService(t, repo, signer)

	_, err := svc.InitResumableUpload(context.Background(), services.InitResumableUploadInput{
		UserID:          userID,
		ContentType:     "video/mp4",
		ContentMD5Hex:   strings.Repeat("c", 32),
		Title:           "Title",
		Description:     "Description",
		DurationSeconds: 10,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if kerr := kerrors.FromError(err); kerr == nil || kerr.Reason != videov1.ErrorReason_ERROR_REASON_UPLOAD_ALREADY_COMPLETED.String() {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestUploadService_InvalidContentType(t *testing.T) {
	repo := &stubUploadRepo{}
	signer := stubSigner{url: "https://signed", expires: time.Now().Add(time.Minute)}
	svc := newUploadService(t, repo, signer)

	_, err := svc.InitResumableUpload(context.Background(), services.InitResumableUploadInput{
		UserID:          uuid.New(),
		ContentType:     "text/plain",
		ContentMD5Hex:   strings.Repeat("d", 32),
		Title:           "Title",
		Description:     "Description",
		DurationSeconds: 10,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if kerr := kerrors.FromError(err); kerr == nil || kerr.Reason != videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String() {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func newUploadService(t *testing.T, repo *stubUploadRepo, signer stubSigner) *services.UploadService {
	t.Helper()
	svc, err := services.NewUploadService(repo, signer, "bucket", 5*time.Minute, log.NewStdLogger(ioDiscard{}))
	if err != nil {
		t.Fatalf("NewUploadService: %v", err)
	}
	return svc
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func ptr(val string) *string { return &val }

func ptrTime(t time.Time) *time.Time { return &t }
