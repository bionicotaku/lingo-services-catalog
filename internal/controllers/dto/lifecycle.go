// Package dto contains transport-layer conversions for lifecycle RPCs.
package dto

import (
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/google/uuid"
)

// ToRegisterUploadInput converts a RegisterUploadRequest into service-layer input with metadata.
func ToRegisterUploadInput(req *videov1.RegisterUploadRequest, meta metadata.HandlerMetadata) (services.RegisterUploadInput, error) {
	uploaderID, err := uuid.Parse(req.GetUploadUserId())
	if err != nil {
		return services.RegisterUploadInput{}, fmt.Errorf("invalid upload_user_id: %w", err)
	}
	input := services.RegisterUploadInput{
		UploadUserID:     uploaderID,
		Title:            req.GetTitle(),
		RawFileReference: req.GetRawFileReference(),
		IdempotencyKey:   meta.IdempotencyKey,
	}
	if req.Description != nil {
		value := req.GetDescription()
		input.Description = &value
	}
	return input, nil
}

// ToUpdateOriginalMediaInput converts UpdateOriginalMediaRequest into a service input.
func ToUpdateOriginalMediaInput(req *videov1.UpdateOriginalMediaRequest, meta metadata.HandlerMetadata) (services.UpdateOriginalMediaInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.UpdateOriginalMediaInput{}, fmt.Errorf("invalid video_id: %w", err)
	}
	input := services.UpdateOriginalMediaInput{
		VideoID:        videoID,
		IdempotencyKey: meta.IdempotencyKey,
	}
	if req.RawFileSize != nil {
		value := req.GetRawFileSize()
		input.RawFileSize = &value
	}
	if req.RawResolution != nil {
		value := req.GetRawResolution()
		input.RawResolution = &value
	}
	if req.RawBitrate != nil {
		value := req.GetRawBitrate()
		input.RawBitrate = &value
	}
	if req.ExpectedVersion != nil {
		value := req.GetExpectedVersion()
		input.ExpectedVersion = &value
	}
	return input, nil
}

// ToUpdateProcessingStatusInput converts UpdateProcessingStatusRequest into a service input.
func ToUpdateProcessingStatusInput(req *videov1.UpdateProcessingStatusRequest, meta metadata.HandlerMetadata) (services.UpdateProcessingStatusInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.UpdateProcessingStatusInput{}, fmt.Errorf("invalid video_id: %w", err)
	}
	stage, err := parseProcessingStage(req.GetStage())
	if err != nil {
		return services.UpdateProcessingStatusInput{}, err
	}
	newStatus, err := parseStageStatus(req.GetNewStatus())
	if err != nil {
		return services.UpdateProcessingStatusInput{}, err
	}
	emittedAt, err := parseTimeRFC3339(req.GetEmittedAt())
	if err != nil {
		return services.UpdateProcessingStatusInput{}, fmt.Errorf("invalid emitted_at: %w", err)
	}
	input := services.UpdateProcessingStatusInput{
		VideoID:        videoID,
		Stage:          stage,
		NewStatus:      newStatus,
		JobID:          req.GetJobId(),
		EmittedAt:      emittedAt,
		ErrorMessage:   optionalStringPointer(req.ErrorMessage),
		IdempotencyKey: meta.IdempotencyKey,
	}
	if req.ExpectedStatus != nil {
		status, err := parseStageStatus(req.GetExpectedStatus())
		if err != nil {
			return services.UpdateProcessingStatusInput{}, fmt.Errorf("invalid expected_status: %w", err)
		}
		input.ExpectedStatus = &status
	}
	if req.ExpectedVersion != nil {
		value := req.GetExpectedVersion()
		input.ExpectedVersion = &value
	}
	return input, nil
}

// ToUpdateMediaInfoInput converts UpdateMediaInfoRequest into a service input.
func ToUpdateMediaInfoInput(req *videov1.UpdateMediaInfoRequest, meta metadata.HandlerMetadata) (services.UpdateMediaInfoInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.UpdateMediaInfoInput{}, fmt.Errorf("invalid video_id: %w", err)
	}
	emittedAt, err := parseTimeRFC3339(req.GetEmittedAt())
	if err != nil {
		return services.UpdateMediaInfoInput{}, fmt.Errorf("invalid emitted_at: %w", err)
	}
	input := services.UpdateMediaInfoInput{
		VideoID:           videoID,
		DurationMicros:    optionalInt64Pointer(req.DurationMicros),
		EncodedResolution: optionalStringPointer(req.EncodedResolution),
		EncodedBitrate:    optionalInt32Pointer(req.EncodedBitrate),
		ThumbnailURL:      optionalStringPointer(req.ThumbnailUrl),
		HLSMasterPlaylist: optionalStringPointer(req.HlsMasterPlaylist),
		IdempotencyKey:    meta.IdempotencyKey,
		JobID:             req.GetJobId(),
		EmittedAt:         emittedAt,
	}
	if req.MediaStatus != nil {
		status, err := parseStageStatus(req.GetMediaStatus())
		if err != nil {
			return services.UpdateMediaInfoInput{}, fmt.Errorf("invalid media_status: %w", err)
		}
		input.MediaStatus = &status
	}
	if req.ExpectedVersion != nil {
		value := req.GetExpectedVersion()
		input.ExpectedVersion = &value
	}
	return input, nil
}

// ToUpdateAIAttributesInput converts UpdateAIAttributesRequest into a service input.
func ToUpdateAIAttributesInput(req *videov1.UpdateAIAttributesRequest, meta metadata.HandlerMetadata) (services.UpdateAIAttributesInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.UpdateAIAttributesInput{}, fmt.Errorf("invalid video_id: %w", err)
	}
	emittedAt, err := parseTimeRFC3339(req.GetEmittedAt())
	if err != nil {
		return services.UpdateAIAttributesInput{}, fmt.Errorf("invalid emitted_at: %w", err)
	}
	input := services.UpdateAIAttributesInput{
		VideoID:        videoID,
		Difficulty:     optionalStringPointer(req.Difficulty),
		Summary:        optionalStringPointer(req.Summary),
		RawSubtitleURL: optionalStringPointer(req.RawSubtitleUrl),
		ErrorMessage:   optionalStringPointer(req.ErrorMessage),
		IdempotencyKey: meta.IdempotencyKey,
		JobID:          req.GetJobId(),
		EmittedAt:      emittedAt,
		Tags:           append([]string(nil), req.GetTags()...),
	}
	if req.AnalysisStatus != nil {
		status, err := parseStageStatus(req.GetAnalysisStatus())
		if err != nil {
			return services.UpdateAIAttributesInput{}, fmt.Errorf("invalid analysis_status: %w", err)
		}
		input.AnalysisStatus = &status
	}
	if req.ExpectedVersion != nil {
		value := req.GetExpectedVersion()
		input.ExpectedVersion = &value
	}
	return input, nil
}

// ToArchiveVideoInput converts ArchiveVideoRequest into a service input.
func ToArchiveVideoInput(req *videov1.ArchiveVideoRequest, meta metadata.HandlerMetadata) (services.ArchiveVideoInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.ArchiveVideoInput{}, fmt.Errorf("invalid video_id: %w", err)
	}
	input := services.ArchiveVideoInput{
		VideoID:        videoID,
		Reason:         optionalStringPointer(req.Reason),
		IdempotencyKey: meta.IdempotencyKey,
	}
	if req.ExpectedVersion != nil {
		value := req.GetExpectedVersion()
		input.ExpectedVersion = &value
	}
	return input, nil
}

// NewVideoRevisionMessage maps a service-layer revision into the protobuf message.
func NewVideoRevisionMessage(revision *services.VideoRevision) *videov1.VideoRevision {
	if revision == nil {
		return &videov1.VideoRevision{}
	}
	return &videov1.VideoRevision{
		VideoId:        revision.VideoID.String(),
		Status:         string(revision.Status),
		MediaStatus:    string(revision.MediaStatus),
		AnalysisStatus: string(revision.AnalysisStatus),
		Version:        revision.Version,
		UpdatedAt:      FormatTime(revision.UpdatedAt),
		EventId:        revision.EventID.String(),
		OccurredAt:     FormatTime(revision.OccurredAt),
	}
}

func parseProcessingStage(stage videov1.ProcessingStage) (services.ProcessingStage, error) {
	switch stage {
	case videov1.ProcessingStage_PROCESSING_STAGE_MEDIA:
		return services.ProcessingStageMedia, nil
	case videov1.ProcessingStage_PROCESSING_STAGE_ANALYSIS:
		return services.ProcessingStageAnalysis, nil
	default:
		return "", fmt.Errorf("unknown processing stage: %s", stage.String())
	}
}

func parseStageStatus(raw string) (po.StageStatus, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch po.StageStatus(value) {
	case po.StagePending, po.StageProcessing, po.StageReady, po.StageFailed:
		return po.StageStatus(value), nil
	default:
		return "", fmt.Errorf("invalid stage status: %s", raw)
	}
}

func parseTimeRFC3339(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("timestamp required")
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func optionalStringPointer(ptr *string) *string {
	if ptr == nil {
		return nil
	}
	value := ptr
	return value
}

func optionalInt64Pointer(ptr *int64) *int64 {
	if ptr == nil {
		return nil
	}
	value := ptr
	return value
}

func optionalInt32Pointer(ptr *int32) *int32 {
	if ptr == nil {
		return nil
	}
	value := ptr
	return value
}
