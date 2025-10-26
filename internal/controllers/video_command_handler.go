package controllers

import (
	"context"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers/mapper"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/internal/views"

	"github.com/go-kratos/kratos/v2/errors"
)

const commandTimeout = 5 * time.Second

// VideoCommandHandler 处理视频写模型相关的 gRPC 请求。
type VideoCommandHandler struct {
	videov1.UnimplementedVideoCommandServiceServer

	svc *services.VideoCommandService
}

// NewVideoCommandHandler 构造命令 Handler。
func NewVideoCommandHandler(svc *services.VideoCommandService) *VideoCommandHandler {
	return &VideoCommandHandler{svc: svc}
}

// CreateVideo 实现 VideoCommandService.CreateVideo RPC。
func (h *VideoCommandHandler) CreateVideo(ctx context.Context, req *videov1.CreateVideoRequest) (*videov1.CreateVideoResponse, error) {
	input, err := mapper.ToCreateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	created, err := h.svc.CreateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return views.NewCreateVideoResponse(created), nil
}

// UpdateVideo 实现 VideoCommandService.UpdateVideo RPC。
func (h *VideoCommandHandler) UpdateVideo(ctx context.Context, req *videov1.UpdateVideoRequest) (*videov1.UpdateVideoResponse, error) {
	input, err := mapper.ToUpdateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	updated, err := h.svc.UpdateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return views.NewUpdateVideoResponse(updated), nil
}

// DeleteVideo 实现 VideoCommandService.DeleteVideo RPC。
func (h *VideoCommandHandler) DeleteVideo(ctx context.Context, req *videov1.DeleteVideoRequest) (*videov1.DeleteVideoResponse, error) {
	input, err := mapper.ToDeleteVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	deleted, err := h.svc.DeleteVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return views.NewDeleteVideoResponse(deleted), nil
}
