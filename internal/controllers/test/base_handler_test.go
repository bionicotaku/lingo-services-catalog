package controllers_test

import (
	"context"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	"google.golang.org/grpc/metadata"
)

func TestBaseHandlerExtractMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-md-global-user-id", "user-123",
		"x-md-idempotency-key", "req-456",
		"x-md-if-match", "etag-1",
		"x-md-if-none-match", "etag-0",
	))

	handler := controllers.NewBaseHandler(controllers.HandlerTimeouts{})
	meta := handler.ExtractMetadata(ctx)

	if meta.UserID != "user-123" {
		t.Fatalf("expected user id to be user-123, got %q", meta.UserID)
	}
	if meta.IdempotencyKey != "req-456" {
		t.Fatalf("expected idempotency key req-456, got %q", meta.IdempotencyKey)
	}
	if meta.IfMatch != "etag-1" {
		t.Fatalf("expected If-Match etag-1, got %q", meta.IfMatch)
	}
	if meta.IfNoneMatch != "etag-0" {
		t.Fatalf("expected If-None-Match etag-0, got %q", meta.IfNoneMatch)
	}

	newCtx := controllers.InjectHandlerMetadata(ctx, meta)
	stored, ok := controllers.HandlerMetadataFromContext(newCtx)
	if !ok {
		t.Fatalf("expected metadata in context")
	}
	if stored != meta {
		t.Fatalf("stored metadata mismatch: %+v vs %+v", stored, meta)
	}
}

func TestBaseHandlerWithTimeout(t *testing.T) {
	handler := controllers.NewBaseHandler(controllers.HandlerTimeouts{Command: 200 * time.Millisecond})
	ctx, cancel := handler.WithTimeout(context.Background(), controllers.HandlerTypeCommand)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline to be set")
	}
	remaining := time.Until(deadline)
	if remaining < 150*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("expected timeout near 200ms, got %v", remaining)
	}
}
