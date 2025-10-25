package events_test

import (
	"errors"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/events"
	"github.com/bionicotaku/kratos-template/internal/models/po"
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

	evt, err := events.NewVideoCreatedEvent(video, evtID, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.GetEventType() != videov1.EventType_EVENT_TYPE_VIDEO_CREATED {
		t.Fatalf("unexpected event type: %v", evt.GetEventType())
	}
	if evt.GetAggregateId() != video.VideoID.String() {
		t.Fatalf("aggregate mismatch")
	}
	if evt.GetOccurredAt().AsTime() != now {
		t.Fatalf("occurred_at mismatch")
	}
	if evt.GetVersion() == 0 {
		t.Fatalf("expected version to be set")
	}
}

func TestNewVideoCreatedEvent_NilVideo(t *testing.T) {
	_, err := events.NewVideoCreatedEvent(nil, uuid.New(), time.Now())
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
	evt, err := events.NewVideoCreatedEvent(video, uuid.New(), now)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	attrs := events.BuildAttributes(evt, events.SchemaVersionV1, "trace123")
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
	changes := events.VideoUpdateChanges{
		Title:  &newTitle,
		Status: &newStatus,
	}
	eventID := uuid.New()

	evt, err := events.NewVideoUpdatedEvent(video, changes, eventID, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.GetEventType() != videov1.EventType_EVENT_TYPE_VIDEO_UPDATED {
		t.Fatalf("unexpected event type: %v", evt.GetEventType())
	}
	payload := evt.GetUpdated()
	if payload.GetTitle().GetValue() != newTitle {
		t.Fatalf("title not populated")
	}
	if payload.GetStatus().GetValue() != string(newStatus) {
		t.Fatalf("status mismatch")
	}
}

func TestNewVideoDeletedEvent(t *testing.T) {
	now := time.Now().UTC()
	video := &po.Video{
		VideoID: uuid.New(),
	}
	reason := "cleanup"
	eventID := uuid.New()

	evt, err := events.NewVideoDeletedEvent(video, eventID, now, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.GetEventType() != videov1.EventType_EVENT_TYPE_VIDEO_DELETED {
		t.Fatalf("unexpected event type: %v", evt.GetEventType())
	}
	payload := evt.GetDeleted()
	if payload.GetReason().GetValue() != reason {
		t.Fatalf("reason mismatch")
	}
	if payload.GetVersion() == 0 {
		t.Fatalf("expected version to be set")
	}
}

func TestNewVideoUpdatedEvent_EmptyChanges(t *testing.T) {
	video := &po.Video{
		VideoID: uuid.New(),
	}
	_, err := events.NewVideoUpdatedEvent(video, events.VideoUpdateChanges{}, uuid.New(), time.Now())
	if !errors.Is(err, events.ErrEmptyUpdatePayload) {
		t.Fatalf("expected ErrEmptyUpdatePayload, got %v", err)
	}
}
