package outboxevents_test

import (
	"errors"
	"testing"
	"time"

	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/google/uuid"
)

func TestNewVideoCreatedEvent(t *testing.T) {
	now := time.Date(2025, 10, 24, 12, 0, 0, 0, time.UTC)
	video := &po.Video{
		VideoID:        uuid.New(),
		UploadUserID:   uuid.New(),
		Title:          "Test",
		Status:         po.VideoStatusPendingUpload,
		MediaStatus:    po.StagePending,
		AnalysisStatus: po.StagePending,
	}
	evtID := uuid.New()

	evt, err := outboxevents.NewVideoCreatedEvent(video, evtID, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoCreated {
		t.Fatalf("unexpected event kind: %v", evt.Kind)
	}
	if evt.AggregateID != video.VideoID {
		t.Fatalf("aggregate mismatch")
	}
	if !evt.OccurredAt.Equal(now.UTC()) {
		t.Fatalf("occurred_at mismatch: got %s want %s", evt.OccurredAt, now.UTC())
	}
	if evt.Version == 0 {
		t.Fatalf("expected version to be set")
	}
	payload, ok := evt.Payload.(*outboxevents.VideoCreated)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.Title != video.Title {
		t.Fatalf("title mismatch")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("encode proto: %v", err)
	}
	if pb.GetCreated().GetTitle() != video.Title {
		t.Fatalf("proto title mismatch")
	}
}

func TestNewVideoCreatedEvent_NilVideo(t *testing.T) {
	_, err := outboxevents.NewVideoCreatedEvent(nil, uuid.New(), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildAttributes(t *testing.T) {
	now := time.Now()
	video := &po.Video{
		VideoID:        uuid.New(),
		UploadUserID:   uuid.New(),
		Title:          "Test",
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
	}
	evt, err := outboxevents.NewVideoCreatedEvent(video, uuid.New(), now)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	attrs := outboxevents.BuildAttributes(evt, outboxevents.SchemaVersionV1, "trace123")
	if attrs["event_type"] != "video.created" {
		t.Fatalf("unexpected event_type: %s", attrs["event_type"])
	}
	if attrs["trace_id"] != "trace123" {
		t.Fatalf("trace id missing")
	}
}

func TestNewVideoUpdatedEvent(t *testing.T) {
	now := time.Now().UTC()
	video := &po.Video{
		VideoID:        uuid.New(),
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		UpdatedAt:      now,
	}
	newTitle := "New Title"
	newStatus := po.VideoStatusPublished
	changes := outboxevents.VideoUpdateChanges{
		Title:  &newTitle,
		Status: &newStatus,
	}
	eventID := uuid.New()

	evt, err := outboxevents.NewVideoUpdatedEvent(video, changes, eventID, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoUpdated {
		t.Fatalf("unexpected event kind: %v", evt.Kind)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoUpdated)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.Title == nil || *payload.Title != newTitle {
		t.Fatalf("title not populated")
	}
	if payload.Status == nil || *payload.Status != string(newStatus) {
		t.Fatalf("status mismatch")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("encode proto: %v", err)
	}
	if pb.GetUpdated().GetTitle() != newTitle {
		t.Fatalf("proto title mismatch")
	}
}

func TestNewVideoDeletedEvent(t *testing.T) {
	now := time.Now().UTC()
	video := &po.Video{
		VideoID: uuid.New(),
	}
	reason := "cleanup"
	eventID := uuid.New()

	evt, err := outboxevents.NewVideoDeletedEvent(video, eventID, now, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoDeleted {
		t.Fatalf("unexpected event kind: %v", evt.Kind)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoDeleted)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.Reason == nil || *payload.Reason != reason {
		t.Fatalf("reason mismatch")
	}
	if payload.DeletedAt == nil || !payload.DeletedAt.Equal(evt.OccurredAt) {
		t.Fatalf("deleted_at mismatch")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("encode proto: %v", err)
	}
	if pb.GetDeleted().GetReason() != reason {
		t.Fatalf("proto reason mismatch")
	}
}

func TestNewVideoUpdatedEvent_EmptyChanges(t *testing.T) {
	video := &po.Video{
		VideoID: uuid.New(),
	}
	_, err := outboxevents.NewVideoUpdatedEvent(video, outboxevents.VideoUpdateChanges{}, uuid.New(), time.Now())
	if !errors.Is(err, outboxevents.ErrEmptyUpdatePayload) {
		t.Fatalf("expected ErrEmptyUpdatePayload, got %v", err)
	}
}
