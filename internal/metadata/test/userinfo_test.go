package metadata_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
)

func TestExtractUserIDFromUserInfo_SupabasePayload(t *testing.T) {
	claims := map[string]any{
		"aud":   "authenticated",
		"exp":   1700000000,
		"email": "studious@example.com",
		"sub":   "f2c9f4f8-4a4b-4e28-9c5b-4d3b2190f155",
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(payload)

	userID, err := metadata.ExtractUserIDFromUserInfo(header)
	if err != nil {
		t.Fatalf("extract user id: %v", err)
	}
	if userID != claims["sub"] {
		t.Fatalf("expected sub %q, got %q", claims["sub"], userID)
	}
}

func TestExtractUserIDFromUserInfo_UserIDFallback(t *testing.T) {
	claims := map[string]any{
		"user_id": "auth0|abc123",
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(payload)

	userID, err := metadata.ExtractUserIDFromUserInfo(header)
	if err != nil {
		t.Fatalf("extract user id: %v", err)
	}
	if userID != claims["user_id"] {
		t.Fatalf("expected fallback user_id %q, got %q", claims["user_id"], userID)
	}
}
