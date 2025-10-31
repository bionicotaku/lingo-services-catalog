package controllers_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

type handlerRepoStub struct {
	existing *po.UploadSession
	session  *po.UploadSession
}

func (s *handlerRepoStub) Upsert(_ context.Context, _ txmanager.Session, input repositories.UpsertUploadInput) (*po.UploadSession, bool, error) {
	sess := &po.UploadSession{
		VideoID:            input.VideoID,
		UserID:             input.UserID,
		Bucket:             input.Bucket,
		ObjectName:         input.ObjectName,
		SignedURL:          input.SignedURL,
		SignedURLExpiresAt: input.SignedURLExpiresAt,
		Status:             po.UploadStatusUploading,
	}
	s.session = sess
	return sess, true, nil
}

func (s *handlerRepoStub) GetByUserMD5(_ context.Context, _ txmanager.Session, _ uuid.UUID, _ string) (*po.UploadSession, error) {
	if s.existing == nil {
		return nil, repositories.ErrUploadNotFound
	}
	return s.existing, nil
}

type handlerSignerStub struct {
	url string
	exp time.Time
}

func (s handlerSignerStub) SignedResumableInitURL(_ context.Context, _ string, _ string, _ string, _ time.Duration) (string, time.Time, error) {
	return s.url, s.exp, nil
}

func TestUploadHandler_Success(t *testing.T) {
	repo := &handlerRepoStub{}
	signer := handlerSignerStub{url: "https://signed.example", exp: time.Now().Add(5 * time.Minute)}
	svc := newUploadServiceForHandler(t, repo, signer)
	handler := controllers.NewUploadHandler(controllers.NewBaseHandler(controllers.HandlerTimeouts{}), svc)

	ctx := incomingContextWithUser(t, uuid.New())
	req := &videov1.InitResumableUploadRequest{
		SizeBytes:       1024,
		ContentType:     "video/mp4",
		ContentMd5Hex:   strings.Repeat("a", 32),
		DurationSeconds: 30,
		Title:           "Title",
		Description:     "Desc",
	}

	resp, err := handler.InitResumableUpload(ctx, req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp.GetResumableInitUrl() == "" {
		t.Fatal("expected signed url")
	}
}

func TestUploadHandler_MissingUserMetadata(t *testing.T) {
	svc := newUploadServiceForHandler(t, &handlerRepoStub{}, handlerSignerStub{url: "https://signed", exp: time.Now().Add(time.Minute)})
	handler := controllers.NewUploadHandler(controllers.NewBaseHandler(controllers.HandlerTimeouts{}), svc)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs())
	_, err := handler.InitResumableUpload(ctx, &videov1.InitResumableUploadRequest{
		ContentType:     "video/mp4",
		ContentMd5Hex:   strings.Repeat("a", 32),
		DurationSeconds: 10,
		Title:           "Title",
		Description:     "Desc",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func newUploadServiceForHandler(t *testing.T, repo *handlerRepoStub, signer handlerSignerStub) *services.UploadService {
	t.Helper()
	svc, err := services.NewUploadService(repo, signer, "bucket", 5*time.Minute, log.NewStdLogger(discardWriter{}))
	if err != nil {
		t.Fatalf("NewUploadService: %v", err)
	}
	return svc
}

func incomingContextWithUser(t *testing.T, user uuid.UUID) context.Context {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"sub": user.String()})
	if err != nil {
		t.Fatalf("marshal user info: %v", err)
	}
	md := metadata.Pairs("x-apigateway-api-userinfo", base64.RawURLEncoding.EncodeToString(payload))
	return metadata.NewIncomingContext(context.Background(), md)
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
