package services

import (
	"context"

	"github.com/google/uuid"
)

// LifecycleService 聚合生命周期相关用例，为控制器提供统一入口。
type LifecycleService struct {
	register   *RegisterUploadService
	original   *OriginalMediaService
	processing *ProcessingStatusService
	media      *MediaInfoService
	ai         *AIAttributesService
	visibility *VisibilityService
}

// NewLifecycleService 构造生命周期聚合服务。
func NewLifecycleService(
	register *RegisterUploadService,
	original *OriginalMediaService,
	processing *ProcessingStatusService,
	media *MediaInfoService,
	ai *AIAttributesService,
	visibility *VisibilityService,
) *LifecycleService {
	return &LifecycleService{
		register:   register,
		original:   original,
		processing: processing,
		media:      media,
		ai:         ai,
		visibility: visibility,
	}
}

// RegisterUpload 调用上传注册用例。
func (s *LifecycleService) RegisterUpload(ctx context.Context, input RegisterUploadInput) (*VideoRevision, error) {
	return s.register.RegisterUpload(ctx, input)
}

// UpdateOriginalMedia 写入原始媒体属性。
func (s *LifecycleService) UpdateOriginalMedia(ctx context.Context, input UpdateOriginalMediaInput) (*VideoRevision, error) {
	return s.original.UpdateOriginalMedia(ctx, input)
}

// UpdateProcessingStatus 推进媒体/AI 流程状态。
func (s *LifecycleService) UpdateProcessingStatus(ctx context.Context, input UpdateProcessingStatusInput) (*VideoRevision, error) {
	return s.processing.UpdateProcessingStatus(ctx, input)
}

// UpdateMediaInfo 写入媒体产物。
func (s *LifecycleService) UpdateMediaInfo(ctx context.Context, input UpdateMediaInfoInput) (*VideoRevision, error) {
	return s.media.UpdateMediaInfo(ctx, input)
}

// UpdateAIAttributes 写入 AI 语义属性。
func (s *LifecycleService) UpdateAIAttributes(ctx context.Context, input UpdateAIAttributesInput) (*VideoRevision, error) {
	return s.ai.UpdateAIAttributes(ctx, input)
}

// ArchiveVideo 归档视频并更新可见性。
func (s *LifecycleService) ArchiveVideo(ctx context.Context, input ArchiveVideoInput) (*VideoRevision, error) {
	return s.visibility.UpdateVisibility(ctx, UpdateVisibilityInput{
		VideoID:         input.VideoID,
		Action:          VisibilityArchive,
		Reason:          input.Reason,
		ExpectedVersion: input.ExpectedVersion,
		IdempotencyKey:  input.IdempotencyKey,
	})
}

// ArchiveVideoInput 描述归档请求的参数。
type ArchiveVideoInput struct {
	VideoID         uuid.UUID
	Reason          *string
	ExpectedVersion *int64
	IdempotencyKey  string
}
