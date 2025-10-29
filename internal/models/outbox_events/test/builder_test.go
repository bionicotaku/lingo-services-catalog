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
	if attrs["event_type"] != "catalog.video.created" {
		t.Fatalf("unexpected event_type: %s", attrs["event_type"])
	}
	if attrs["trace_id"] != "trace123" {
		t.Fatalf("trace id missing")
	}
}

func TestNewVideoUpdatedEvent(t *testing.T) {
	now := time.Now().UTC()
	video := &po.Video{
		VideoID:          uuid.New(),
		Status:           po.VideoStatusReady,
		MediaStatus:      po.StageReady,
		AnalysisStatus:   po.StageReady,
		VisibilityStatus: po.VisibilityPublic,
		PublishAt:        &now,
		UpdatedAt:        now,
	}
	newTitle := "New Title"
	newStatus := po.VideoStatusPublished
	visibility := po.VisibilityPublic
	changes := outboxevents.VideoUpdateChanges{
		Title:            &newTitle,
		Status:           &newStatus,
		VisibilityStatus: &visibility,
		PublishAt:        &now,
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
	if payload.VisibilityStatus == nil || *payload.VisibilityStatus != visibility {
		t.Fatalf("visibility status mismatch")
	}
	if payload.PublishedAt == nil {
		t.Fatalf("published_at missing")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("encode proto: %v", err)
	}
	if pb.GetUpdated().GetTitle() != newTitle {
		t.Fatalf("proto title mismatch")
	}
	if pb.GetUpdated().GetVisibilityStatus() != visibility {
		t.Fatalf("proto visibility mismatch")
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

func TestNewVideoMediaReadyEvent(t *testing.T) {
	emittedAt := time.Date(2025, 10, 24, 14, 0, 0, 0, time.UTC)
	jobID := "job-media-123"
	video := &po.Video{
		VideoID:           uuid.New(),
		Status:            po.VideoStatusReady,
		MediaStatus:       po.StageReady,
		AnalysisStatus:    po.StageProcessing,
		DurationMicros:    ptrInt64(120 * 1_000_000),
		EncodedResolution: ptrString("1080p"),
		EncodedBitrate:    ptrInt32(3200),
		ThumbnailURL:      ptrString("https://example.com/thumb.jpg"),
		HLSMasterPlaylist: ptrString("https://example.com/playlist.m3u8"),
		MediaJobID:        &jobID,
		MediaEmittedAt:    &emittedAt,
		UpdatedAt:         emittedAt,
	}

	evt, err := outboxevents.NewVideoMediaReadyEvent(video, uuid.New(), emittedAt)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoMediaReady {
		t.Fatalf("unexpected kind: %v", evt.Kind)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoMediaReady)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.JobID == nil || *payload.JobID != jobID {
		t.Fatalf("job id mismatch")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("ToProto: %v", err)
	}
	if pb.GetMediaReady().GetEncodedResolution() != "1080p" {
		t.Fatalf("encoded resolution mismatch")
	}
	if pb.GetMediaReady().GetAnalysisStatus() != string(po.StageProcessing) {
		t.Fatalf("analysis status mismatch")
	}
}

func TestNewVideoAIEnrichedEvent(t *testing.T) {
	emittedAt := time.Date(2025, 10, 25, 9, 30, 0, 0, time.UTC)
	jobID := "job-ai-789"
	diff := "intermediate"
	summary := "summary"
	errorMessage := "previous error"
	video := &po.Video{
		VideoID:           uuid.New(),
		Status:            po.VideoStatusReady,
		MediaStatus:       po.StageReady,
		AnalysisStatus:    po.StageReady,
		Difficulty:        &diff,
		Summary:           &summary,
		Tags:              []string{"tag1", "tag2"},
		RawSubtitleURL:    ptrString("https://example.com/sub.vtt"),
		AnalysisJobID:     &jobID,
		AnalysisEmittedAt: &emittedAt,
		ErrorMessage:      &errorMessage,
		UpdatedAt:         emittedAt,
	}

	evt, err := outboxevents.NewVideoAIEnrichedEvent(video, uuid.New(), emittedAt)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoAIEnriched {
		t.Fatalf("unexpected kind: %v", evt.Kind)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoAIEnriched)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if len(payload.Tags) != 2 {
		t.Fatalf("tags not copied")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("ToProto: %v", err)
	}
	if pb.GetAiEnriched().GetJobId() != jobID {
		t.Fatalf("job id mismatch")
	}
	if pb.GetAiEnriched().GetMediaStatus() != string(po.StageReady) {
		t.Fatalf("media status mismatch")
	}
}

func TestNewVideoProcessingFailedEvent(t *testing.T) {
	emittedAt := time.Date(2025, 10, 25, 10, 0, 0, 0, time.UTC)
	jobID := "job-fail-001"
	errMsg := "transcode failed"
	video := &po.Video{
		VideoID:        uuid.New(),
		Status:         po.VideoStatusFailed,
		MediaStatus:    po.StageFailed,
		AnalysisStatus: po.StageProcessing,
		UpdatedAt:      emittedAt,
	}

	evt, err := outboxevents.NewVideoProcessingFailedEvent(video, "media", &jobID, &emittedAt, &errMsg, uuid.New(), emittedAt)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if evt.Kind != outboxevents.KindVideoProcessingFailed {
		t.Fatalf("unexpected kind: %v", evt.Kind)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoProcessingFailed)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.ErrorMessage == nil || *payload.ErrorMessage != errMsg {
		t.Fatalf("error message mismatch")
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("ToProto: %v", err)
	}
	if pb.GetProcessingFailed().GetFailedStage() != "media" {
		t.Fatalf("stage mismatch")
	}
}

func TestNewVideoVisibilityChangedEvent(t *testing.T) {
	now := time.Now().UTC()
	reason := "manual publish"
	video := &po.Video{
		VideoID:          uuid.New(),
		Status:           po.VideoStatusPublished,
		VisibilityStatus: po.VisibilityPublic,
		PublishAt:        &now,
		UpdatedAt:        now,
	}

	evt, err := outboxevents.NewVideoVisibilityChangedEvent(video, po.VideoStatusReady, &reason, uuid.New(), now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	payload, ok := evt.Payload.(*outboxevents.VideoVisibilityChanged)
	if !ok {
		t.Fatalf("payload type mismatch: %T", evt.Payload)
	}
	if payload.PreviousStatus == nil || string(*payload.PreviousStatus) != string(po.VideoStatusReady) {
		t.Fatalf("previous status mismatch")
	}
	if payload.PublishedAt == nil {
		t.Fatalf("published_at missing")
	}
	if payload.VisibilityStatus != po.VisibilityPublic {
		t.Fatalf("visibility status mismatch: %s", payload.VisibilityStatus)
	}
	pb, err := outboxevents.ToProto(evt)
	if err != nil {
		t.Fatalf("ToProto: %v", err)
	}
	if pb.GetVisibilityChanged().GetPreviousStatus() != string(po.VideoStatusReady) {
		t.Fatalf("proto previous status mismatch")
	}
	if pb.GetVisibilityChanged().GetVisibilityStatus() != po.VisibilityPublic {
		t.Fatalf("proto visibility status mismatch")
	}
}

func ptrString(v string) *string { return &v }

func ptrInt64(v int64) *int64 { return &v }

func ptrInt32(v int32) *int32 { return &v }
