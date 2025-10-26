package controllers

import (
	"context"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers/dto"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/errors"
)

// VideoCommandHandler 处理视频写模型相关的 gRPC 请求。
type VideoCommandHandler struct {
	videov1.UnimplementedVideoCommandServiceServer

	*BaseHandler
	svc *services.VideoCommandService
}

// NewVideoCommandHandler 构造命令 Handler。
func NewVideoCommandHandler(svc *services.VideoCommandService, base *BaseHandler) *VideoCommandHandler {
	if base == nil {
		base = NewBaseHandler(HandlerTimeouts{})
	}
	return &VideoCommandHandler{BaseHandler: base, svc: svc}
}

// CreateVideo 实现 VideoCommandService.CreateVideo RPC。
func (h *VideoCommandHandler) CreateVideo(ctx context.Context, req *videov1.CreateVideoRequest) (*videov1.CreateVideoResponse, error) {
	input, err := dto.ToCreateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	meta := h.ExtractMetadata(ctx)
	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	created, err := h.svc.CreateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return dto.NewCreateVideoResponse(created), nil
}

// UpdateVideo 实现 VideoCommandService.UpdateVideo RPC。
func (h *VideoCommandHandler) UpdateVideo(ctx context.Context, req *videov1.UpdateVideoRequest) (*videov1.UpdateVideoResponse, error) {
	input, err := dto.ToUpdateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	meta := h.ExtractMetadata(ctx)
	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	updated, err := h.svc.UpdateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return dto.NewUpdateVideoResponse(updated), nil
}

// DeleteVideo 实现 VideoCommandService.DeleteVideo RPC。
func (h *VideoCommandHandler) DeleteVideo(ctx context.Context, req *videov1.DeleteVideoRequest) (*videov1.DeleteVideoResponse, error) {
	input, err := dto.ToDeleteVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	meta := h.ExtractMetadata(ctx)
	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	deleted, err := h.svc.DeleteVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return dto.NewDeleteVideoResponse(deleted), nil
}
