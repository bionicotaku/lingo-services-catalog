package engagement

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDecoderProto(t *testing.T) {
	decoder := newEventDecoder()
	now := time.Date(2025, 10, 26, 12, 0, 0, 0, time.UTC)
	payload, err := proto.Marshal(&EventProto{
		UserId:     "7b61d0ed-1111-4c3e-9d93-aaaaaaaaaaaa",
		VideoId:    "8c22ebce-2222-4e87-bbbb-bbbbbbbbbbbb",
		HasLiked:   proto.Bool(true),
		OccurredAt: timestamppb.New(now),
	})
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}

	evt, err := decoder.Decode(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if evt.UserID != "7b61d0ed-1111-4c3e-9d93-aaaaaaaaaaaa" {
		t.Fatalf("unexpected user id: %s", evt.UserID)
	}
	if evt.Version != EventVersion {
		t.Fatalf("expected default version, got %s", evt.Version)
	}
	if evt.HasLiked == nil || !*evt.HasLiked {
		t.Fatalf("expected has_liked true")
	}
}

func TestDecoderJSON(t *testing.T) {
	decoder := newEventDecoder()
	payload := []byte(`{"user_id":"7b61d0ed-1111-4c3e-9d93-aaaaaaaaaaaa","video_id":"8c22ebce-2222-4e87-bbbb-bbbbbbbbbbbb","has_bookmarked":true,"occurred_at":"2025-10-26T12:00:00Z"}`)

	evt, err := decoder.Decode(payload)
	if err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if evt.HasBookmarked == nil || !*evt.HasBookmarked {
		t.Fatalf("expected bookmarked true")
	}
	if evt.Version != EventVersion {
		t.Fatalf("expected version fallback")
	}
}
