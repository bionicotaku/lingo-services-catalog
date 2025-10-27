package controllers

import (
	"context"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers/dto"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
)

// LifecycleHandler 实现 CatalogLifecycleService gRPC 接口。
type LifecycleHandler struct {
	videov1.UnimplementedCatalogLifecycleServiceServer

	*BaseHandler
	svc *services.LifecycleService
}

// NewLifecycleHandler 构造生命周期 Handler。
func NewLifecycleHandler(svc *services.LifecycleService, base *BaseHandler) *LifecycleHandler {
	if base == nil {
		base = NewBaseHandler(HandlerTimeouts{})
	}
	return &LifecycleHandler{BaseHandler: base, svc: svc}
}

// RegisterUpload 处理上传注册。
func (h *LifecycleHandler) RegisterUpload(ctx context.Context, req *videov1.RegisterUploadRequest) (*videov1.RegisterUploadResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToRegisterUploadInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.RegisterUpload(timeoutCtx, input)
	if err != nil {
		return nil, err
	}

	// 注册流程将 revision.UpdatedAt 作为创建时间返回
	return &videov1.RegisterUploadResponse{
		VideoId:        revision.VideoID.String(),
		Status:         string(revision.Status),
		MediaStatus:    string(revision.MediaStatus),
		AnalysisStatus: string(revision.AnalysisStatus),
		CreatedAt:      dto.FormatTime(revision.UpdatedAt),
		EventId:        revision.EventID.String(),
		Version:        revision.Version,
		OccurredAt:     dto.FormatTime(revision.OccurredAt),
	}, nil
}

// UpdateOriginalMedia 处理原始媒体属性写入。
func (h *LifecycleHandler) UpdateOriginalMedia(ctx context.Context, req *videov1.UpdateOriginalMediaRequest) (*videov1.UpdateOriginalMediaResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToUpdateOriginalMediaInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.UpdateOriginalMedia(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &videov1.UpdateOriginalMediaResponse{Revision: dto.NewVideoRevisionMessage(revision)}, nil
}

// UpdateProcessingStatus 处理阶段状态推进。
func (h *LifecycleHandler) UpdateProcessingStatus(ctx context.Context, req *videov1.UpdateProcessingStatusRequest) (*videov1.UpdateProcessingStatusResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToUpdateProcessingStatusInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.UpdateProcessingStatus(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &videov1.UpdateProcessingStatusResponse{Revision: dto.NewVideoRevisionMessage(revision)}, nil
}

// UpdateMediaInfo 处理媒体产物回写。
func (h *LifecycleHandler) UpdateMediaInfo(ctx context.Context, req *videov1.UpdateMediaInfoRequest) (*videov1.UpdateMediaInfoResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToUpdateMediaInfoInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.UpdateMediaInfo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &videov1.UpdateMediaInfoResponse{Revision: dto.NewVideoRevisionMessage(revision)}, nil
}

// UpdateAIAttributes 处理 AI 属性回写。
func (h *LifecycleHandler) UpdateAIAttributes(ctx context.Context, req *videov1.UpdateAIAttributesRequest) (*videov1.UpdateAIAttributesResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToUpdateAIAttributesInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.UpdateAIAttributes(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &videov1.UpdateAIAttributesResponse{Revision: dto.NewVideoRevisionMessage(revision)}, nil
}

// ArchiveVideo 处理归档请求。
func (h *LifecycleHandler) ArchiveVideo(ctx context.Context, req *videov1.ArchiveVideoRequest) (*videov1.ArchiveVideoResponse, error) {
	meta := h.ExtractMetadata(ctx)
	input, err := dto.ToArchiveVideoInput(req, meta)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	revision, err := h.svc.ArchiveVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &videov1.ArchiveVideoResponse{Revision: dto.NewVideoRevisionMessage(revision)}, nil
}
