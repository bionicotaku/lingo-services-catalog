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

// ProcessingStage 描述待推进的业务阶段。
type ProcessingStage string

const (
	// ProcessingStageMedia 表示媒体处理阶段。
	ProcessingStageMedia ProcessingStage = "media"
	// ProcessingStageAnalysis 表示 AI 分析阶段。
	ProcessingStageAnalysis ProcessingStage = "analysis"
)

// UpdateProcessingStatusInput 输入参数。
type UpdateProcessingStatusInput struct {
	VideoID         uuid.UUID
	Stage           ProcessingStage
	ExpectedStatus  *po.StageStatus
	NewStatus       po.StageStatus
	JobID           string
	EmittedAt       time.Time
	ErrorMessage    *string
	ExpectedVersion *int64
	IdempotencyKey  string
}

// ProcessingStatusService 推进媒体/AI 阶段状态。
type ProcessingStatusService struct {
	writer *LifecycleWriter
	repo   VideoLookupRepo
}

// NewProcessingStatusService 构造 ProcessingStatusService。
func NewProcessingStatusService(writer *LifecycleWriter, repo VideoLookupRepo) *ProcessingStatusService {
	return &ProcessingStatusService{writer: writer, repo: repo}
}

// UpdateProcessingStatus 推进阶段状态，校验 job / emitted_at / expected_status。
func (s *ProcessingStatusService) UpdateProcessingStatus(ctx context.Context, input UpdateProcessingStatusInput) (*VideoRevision, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	if err := validateStageStatus(input.NewStatus); err != nil {
		return nil, err
	}
	if err := validateStage(input.Stage); err != nil {
		return nil, err
	}
	current, err := s.repo.GetLifecycleSnapshot(ctx, nil, input.VideoID)
	if err != nil {
		if errors.Is(err, repositories.ErrVideoNotFound) {
			return nil, ErrVideoNotFound
		}
		return nil, errors.InternalServer(videov1.ErrorReason_ERROR_REASON_QUERY_VIDEO_FAILED.String(), fmt.Sprintf("load video: %v", err))
	}

	if err := s.validateExpectations(input, current); err != nil {
		return nil, err
	}

	updateInput := buildStageUpdateInput(input, current)
	updateInput.IdempotencyKey = input.IdempotencyKey
	updateInput.ExpectedVersion = input.ExpectedVersion

	return s.writer.UpdateVideo(
		ctx,
		updateInput,
		WithPreviousVideo(current),
		WithAdditionalEvents(func(_ context.Context, updated *po.Video, previous *po.Video) ([]*outboxevents.DomainEvent, error) {
			if previous == nil {
				return nil, nil
			}
			if input.NewStatus != po.StageFailed {
				return nil, nil
			}
			var prevStage po.StageStatus
			switch input.Stage {
			case ProcessingStageMedia:
				prevStage = previous.MediaStatus
			case ProcessingStageAnalysis:
				prevStage = previous.AnalysisStatus
			}
			if prevStage == po.StageFailed {
				return nil, nil
			}

			var jobID *string
			var emittedAt *time.Time
			switch input.Stage {
			case ProcessingStageMedia:
				jobID = updated.MediaJobID
				emittedAt = updated.MediaEmittedAt
			case ProcessingStageAnalysis:
				jobID = updated.AnalysisJobID
				emittedAt = updated.AnalysisEmittedAt
			}

			event, err := outboxevents.NewVideoProcessingFailedEvent(
				updated,
				string(input.Stage),
				jobID,
				emittedAt,
				input.ErrorMessage,
				uuid.New(),
				processingOccurredAt(emittedAt, updated),
			)
			if err != nil {
				return nil, err
			}
			return []*outboxevents.DomainEvent{event}, nil
		}),
	)
}

func processingOccurredAt(emittedAt *time.Time, video *po.Video) time.Time {
	if emittedAt != nil {
		return emittedAt.UTC()
	}
	if video != nil && !video.UpdatedAt.IsZero() {
		return video.UpdatedAt.UTC()
	}
	return time.Time{}
}

func validateStage(stage ProcessingStage) error {
	switch stage {
	case ProcessingStageMedia, ProcessingStageAnalysis:
		return nil
	default:
		return errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "unknown processing stage")
	}
}

func validateStageStatus(status po.StageStatus) error {
	switch status {
	case po.StagePending, po.StageProcessing, po.StageReady, po.StageFailed:
		return nil
	default:
		return errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "invalid stage status")
	}
}

func (s *ProcessingStatusService) validateExpectations(input UpdateProcessingStatusInput, current *po.Video) error {
	var (
		currStatus  po.StageStatus
		currJobID   *string
		currEmitted *time.Time
	)

	switch input.Stage {
	case ProcessingStageMedia:
		currStatus = current.MediaStatus
		currJobID = current.MediaJobID
		currEmitted = current.MediaEmittedAt
	case ProcessingStageAnalysis:
		currStatus = current.AnalysisStatus
		currJobID = current.AnalysisJobID
		currEmitted = current.AnalysisEmittedAt
	}

	if input.ExpectedStatus != nil && currStatus != *input.ExpectedStatus {
		return errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "stage status conflict")
	}
	if !input.EmittedAt.IsZero() && currEmitted != nil && input.EmittedAt.UTC().Before(currEmitted.UTC()) {
		return errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "stale emitted_at")
	}
	if input.JobID != "" && currJobID != nil && *currJobID != "" && *currJobID != input.JobID {
		if currEmitted != nil && !input.EmittedAt.IsZero() && !input.EmittedAt.After(currEmitted.UTC()) {
			return errors.Conflict(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "job_id conflict")
		}
	}
	return nil
}

func buildStageUpdateInput(input UpdateProcessingStatusInput, current *po.Video) UpdateVideoInput {
	update := UpdateVideoInput{
		VideoID: input.VideoID,
	}

	occurred := input.EmittedAt.UTC()

	mediaStatus := current.MediaStatus
	analysisStatus := current.AnalysisStatus

	switch input.Stage {
	case ProcessingStageMedia:
		newStatus := input.NewStatus
		update.MediaStatus = &newStatus
		if input.JobID != "" {
			job := input.JobID
			update.MediaJobID = &job
		}
		update.MediaEmittedAt = &occurred
		mediaStatus = input.NewStatus
	case ProcessingStageAnalysis:
		newStatus := input.NewStatus
		update.AnalysisStatus = &newStatus
		if input.JobID != "" {
			job := input.JobID
			update.AnalysisJobID = &job
		}
		update.AnalysisEmittedAt = &occurred
		analysisStatus = input.NewStatus
	}

	computed := computeOverallStatus(current.Status, mediaStatus, analysisStatus, input.NewStatus)
	if computed != current.Status {
		statusValue := computed
		update.Status = &statusValue
	}

	if input.NewStatus == po.StageFailed && input.ErrorMessage != nil {
		update.ErrorMessage = input.ErrorMessage
	}
	return update
}

func computeOverallStatus(current po.VideoStatus, media po.StageStatus, analysis po.StageStatus, latest po.StageStatus) po.VideoStatus {
	if latest == po.StageFailed {
		return po.VideoStatusFailed
	}
	// 若任一阶段仍在处理中，则保持 processing
	if (media == po.StageProcessing || analysis == po.StageProcessing || media == po.StagePending || analysis == po.StagePending) && current != po.VideoStatusPublished {
		return po.VideoStatusProcessing
	}
	if media == po.StageReady && analysis == po.StageReady {
		if current == po.VideoStatusPublished {
			return current
		}
		return po.VideoStatusReady
	}
	if current == po.VideoStatusFailed && latest != po.StageFailed {
		return po.VideoStatusProcessing
	}
	return current
}
