package services

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	outboxevents "github.com/bionicotaku/lingo-services-catalog/internal/models/outbox_events"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// UpdateAIAttributesInput 描述 AI 属性写入所需字段。
type UpdateAIAttributesInput struct {
	VideoID         uuid.UUID
	Difficulty      *string
	Summary         *string
	Tags            []string
	RawSubtitleURL  *string
	AnalysisStatus  *po.StageStatus
	ErrorMessage    *string
	JobID           string
	EmittedAt       time.Time
	ExpectedVersion *int64
	IdempotencyKey  string
}

// AIAttributesService 负责更新 AI 语义字段并重算状态。
type AIAttributesService struct {
	writer *LifecycleWriter
	repo   VideoLookupRepo
}

// NewAIAttributesService 构造 AIAttributesService。
func NewAIAttributesService(writer *LifecycleWriter, repo VideoLookupRepo) *AIAttributesService {
	return &AIAttributesService{writer: writer, repo: repo}
}

// UpdateAIAttributes 写入 AI 语义字段并按需推进分析阶段状态。
func (s *AIAttributesService) UpdateAIAttributes(ctx context.Context, input UpdateAIAttributesInput) (*VideoRevision, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	current, err := s.repo.GetLifecycleSnapshot(ctx, nil, input.VideoID)
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("load video: %v", err))
	}

	analysisStatus := current.AnalysisStatus
	if input.AnalysisStatus != nil {
		analysisStatus = *input.AnalysisStatus
	}
	mediaStatus := current.MediaStatus

	updateInput := UpdateVideoInput{
		VideoID:        input.VideoID,
		Difficulty:     input.Difficulty,
		Summary:        input.Summary,
		RawSubtitleURL: input.RawSubtitleURL,
		ErrorMessage:   input.ErrorMessage,
	}
	if input.AnalysisStatus != nil {
		statusValue := *input.AnalysisStatus
		updateInput.AnalysisStatus = &statusValue
	}
	if len(input.Tags) > 0 {
		updateInput.Tags = append([]string(nil), input.Tags...)
	}
	if input.JobID != "" {
		job := input.JobID
		updateInput.AnalysisJobID = &job
	}
	if !input.EmittedAt.IsZero() {
		emitted := input.EmittedAt.UTC()
		updateInput.AnalysisEmittedAt = &emitted
	}
	updateInput.IdempotencyKey = input.IdempotencyKey
	updateInput.ExpectedVersion = input.ExpectedVersion

	computed := computeOverallStatus(current.Status, mediaStatus, analysisStatus, analysisStatus)
	if computed != current.Status {
		statusValue := computed
		updateInput.Status = &statusValue
	}

	return s.writer.UpdateVideo(
		ctx,
		updateInput,
		WithPreviousVideo(current),
		WithAdditionalEvents(func(_ context.Context, updated *po.Video, previous *po.Video) ([]*outboxevents.DomainEvent, error) {
			if previous == nil {
				return nil, nil
			}
			if updated.AnalysisStatus != po.StageReady {
				return nil, nil
			}
			if previous.AnalysisStatus == po.StageReady && !aiPayloadChanged(previous, updated) {
				return nil, nil
			}
			event, err := outboxevents.NewVideoAIEnrichedEvent(updated, uuid.New(), aiOccurredAt(updated))
			if err != nil {
				return nil, err
			}
			return []*outboxevents.DomainEvent{event}, nil
		}),
	)
}

func aiOccurredAt(video *po.Video) time.Time {
	if video == nil || video.AnalysisEmittedAt == nil {
		return time.Time{}
	}
	return video.AnalysisEmittedAt.UTC()
}

func aiPayloadChanged(previous, updated *po.Video) bool {
	if previous == nil || updated == nil {
		return true
	}
	switch {
	case !equalStringPtr(previous.Difficulty, updated.Difficulty):
		return true
	case !equalStringPtr(previous.Summary, updated.Summary):
		return true
	case !equalStringPtr(previous.RawSubtitleURL, updated.RawSubtitleURL):
		return true
	case !equalStringSlices(previous.Tags, updated.Tags):
		return true
	case !equalStringPtr(previous.ErrorMessage, updated.ErrorMessage):
		return true
	default:
		return false
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
