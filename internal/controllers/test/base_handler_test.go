package controllers_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	"google.golang.org/grpc/metadata"
)

func TestBaseHandlerExtractMetadata(t *testing.T) {
	claims := map[string]any{
		"sub":   "7b61d0ed-5ba1-4f21-a636-7f9f1a9f9a01",
		"email": "user@example.com",
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	headerValue := base64.RawURLEncoding.EncodeToString(payload)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-apigateway-api-userinfo", headerValue,
		"x-md-idempotency-key", "req-456",
		"x-md-if-match", "etag-1",
		"x-md-if-none-match", "etag-0",
	))

	handler := controllers.NewBaseHandler(controllers.HandlerTimeouts{})
	meta := handler.ExtractMetadata(ctx)

	if meta.UserID != claims["sub"] {
		t.Fatalf("expected user id to be %q, got %q", claims["sub"], meta.UserID)
	}
	if meta.RawUserInfo != headerValue {
		t.Fatalf("expected raw userinfo to match header")
	}
	if meta.InvalidUserInfo {
		t.Fatalf("expected user info to be valid")
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

func TestBaseHandlerInvalidUserInfo(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-apigateway-api-userinfo", "!!!invalid!!!",
	))
	handler := controllers.NewBaseHandler(controllers.HandlerTimeouts{})
	meta := handler.ExtractMetadata(ctx)
	if !meta.InvalidUserInfo {
		t.Fatalf("expected invalid user info flag")
	}
	if meta.UserID != "" {
		t.Fatalf("expected empty user id, got %q", meta.UserID)
	}
}
