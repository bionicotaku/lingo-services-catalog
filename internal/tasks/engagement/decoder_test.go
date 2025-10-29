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
		EventName:      "profile.engagement.added",
		EngagementType: "like",
		UserId:         "7b61d0ed-1111-4c3e-9d93-aaaaaaaaaaaa",
		VideoId:        "8c22ebce-2222-4e87-bbbb-bbbbbbbbbbbb",
		OccurredAt:     timestamppb.New(now),
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
	action, err := evt.resolveAction()
	if err != nil {
		t.Fatalf("expected action parse, got %v", err)
	}
	if action != actionAdded {
		t.Fatalf("expected action added, got %s", action)
	}
	kind, err := evt.resolveKind()
	if err != nil {
		t.Fatalf("expected kind parse, got %v", err)
	}
	if kind != kindLike {
		t.Fatalf("expected kind like, got %s", kind)
	}
}

func TestDecoderJSON(t *testing.T) {
	decoder := newEventDecoder()
	payload := []byte(`{"event_name":"profile.engagement.removed","state":"removed","engagement_type":"bookmark","user_id":"7b61d0ed-1111-4c3e-9d93-aaaaaaaaaaaa","video_id":"8c22ebce-2222-4e87-bbbb-bbbbbbbbbbbb","occurred_at":"2025-10-26T12:00:00Z"}`)

	evt, err := decoder.Decode(payload)
	if err != nil {
		t.Fatalf("decode json: %v", err)
	}
	action, err := evt.resolveAction()
	if err != nil {
		t.Fatalf("expected action parse, got %v", err)
	}
	if action != actionRemoved {
		t.Fatalf("expected action removed, got %s", action)
	}
	kind, err := evt.resolveKind()
	if err != nil {
		t.Fatalf("expected kind parse, got %v", err)
	}
	if kind != kindBookmark {
		t.Fatalf("expected kind bookmark, got %s", kind)
	}
	if evt.Version != EventVersion {
		t.Fatalf("expected version fallback")
	}
}
