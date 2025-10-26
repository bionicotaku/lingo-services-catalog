package services_test

import (
	"context"
	"io"
	"testing"

	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

func TestVideoQueryService_ListMyUploadsRequiresUserID(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	svc := services.NewVideoQueryService(&videoRepoStub{}, nil, noopTxManager{}, logger)

	_, _, err := svc.ListMyUploads(context.Background(), 10, "", nil)
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

	_, _, err := svc.ListMyUploads(ctx, 10, "", nil)
	if err == nil {
		t.Fatalf("expected error for invalid user id")
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

	_, err := svc.GetVideoDetail(ctx, uuid.New())
	if err == nil {
		t.Fatalf("expected error for invalid user id metadata")
	}
	e := errors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected http 400, got %d (%s)", e.Code, e.Message)
	}
}
