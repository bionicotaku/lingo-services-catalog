package services

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// UpdateAIAttributesInput 描述 AI 属性写入所需字段。
type UpdateAIAttributesInput struct {
	VideoID        uuid.UUID
	Difficulty     *string
	Summary        *string
	RawSubtitleURL *string
	AnalysisStatus *po.StageStatus
	ErrorMessage   *string
}

// AIAttributesService 负责更新 AI 语义字段并重算状态。
type AIAttributesService struct {
	commands *VideoCommandService
	repo     *repositories.VideoRepository
}

// NewAIAttributesService 构造 AIAttributesService。
func NewAIAttributesService(commands *VideoCommandService, repo *repositories.VideoRepository) *AIAttributesService {
	return &AIAttributesService{commands: commands, repo: repo}
}

// UpdateAIAttributes 写入 AI 语义字段并按需推进分析阶段状态。
func (s *AIAttributesService) UpdateAIAttributes(ctx context.Context, input UpdateAIAttributesInput) (*vo.VideoUpdated, error) {
	if input.VideoID == uuid.Nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), "video_id is required")
	}
	current, err := s.repo.GetByID(ctx, nil, input.VideoID)
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
		statusValue := string(*input.AnalysisStatus)
		updateInput.AnalysisStatus = &statusValue
	}

	computed := computeOverallStatus(current.Status, mediaStatus, analysisStatus, analysisStatus)
	if computed != current.Status {
		statusValue := string(computed)
		updateInput.Status = &statusValue
	}

	return s.commands.UpdateVideo(ctx, updateInput)
}
