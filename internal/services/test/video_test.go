package services_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestCreateVideoEnqueuesOutbox(t *testing.T) {
	repo := &videoRepoStub{video: &po.Video{
		VideoID:        uuid.New(),
		UploadUserID:   uuid.New(),
		Title:          "demo",
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
	}}
	outbox := &outboxRepoStub{}
	logger := log.NewStdLogger(io.Discard)
	uc := services.NewVideoUsecase(repo, outbox, noopTxManager{}, logger)

	created, err := uc.CreateVideo(context.Background(), services.CreateVideoInput{
		UploadUserID:     uuid.New(),
		Title:            "demo",
		RawFileReference: "gs://bucket/object",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil {
		t.Fatalf("expected created response")
	}
	if len(outbox.messages) != 1 {
		t.Fatalf("expected 1 outbox message, got %d", len(outbox.messages))
	}
	if outbox.messages[0].EventType != "video.created" {
		t.Fatalf("unexpected event type: %s", outbox.messages[0].EventType)
	}
}

func TestCreateVideoRepoError(t *testing.T) {
	repo := &videoRepoStub{err: errors.New("db down")}
	outbox := &outboxRepoStub{}
	logger := log.NewStdLogger(io.Discard)

	uc := services.NewVideoUsecase(repo, outbox, noopTxManager{}, logger)
	_, err := uc.CreateVideo(context.Background(), services.CreateVideoInput{
		UploadUserID:     uuid.New(),
		Title:            "demo",
		RawFileReference: "gs://bucket/object",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(outbox.messages) != 0 {
		t.Fatal("outbox should not be called on repo error")
	}
}

func TestCreateVideoOutboxError(t *testing.T) {
	repo := &videoRepoStub{video: &po.Video{
		VideoID:        uuid.New(),
		UploadUserID:   uuid.New(),
		Title:          "demo",
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
	}}
	outbox := &outboxRepoStub{err: errors.New("outbox down")}
	logger := log.NewStdLogger(io.Discard)

	uc := services.NewVideoUsecase(repo, outbox, noopTxManager{}, logger)
	_, err := uc.CreateVideo(context.Background(), services.CreateVideoInput{
		UploadUserID:     uuid.New(),
		Title:            "demo",
		RawFileReference: "gs://bucket/object",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---- stubs ----

type videoRepoStub struct {
	video *po.Video
	err   error
}

func (s *videoRepoStub) Create(_ context.Context, _ txmanager.Session, _ repositories.CreateVideoInput) (*po.Video, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.video, nil
}

func (s *videoRepoStub) FindByID(_ context.Context, _ txmanager.Session, _ uuid.UUID) (*po.VideoReadyView, error) {
	return nil, repositories.ErrVideoNotFound
}

type outboxRepoStub struct {
	messages []repositories.OutboxMessage
	err      error
}

func (s *outboxRepoStub) Enqueue(_ context.Context, _ txmanager.Session, msg repositories.OutboxMessage) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, msg)
	return nil
}

type noopTxManager struct{}

type noopSession struct{}

func (noopSession) Tx() pgx.Tx               { return nil }
func (noopSession) Context() context.Context { return context.Background() }

func (noopTxManager) WithinTx(ctx context.Context, _ txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	return fn(ctx, noopSession{})
}

func (noopTxManager) WithinReadOnlyTx(ctx context.Context, _ txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	return fn(ctx, noopSession{})
}
