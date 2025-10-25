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

const (
	// queryTimeout 定义查询操作的默认超时时间
	queryTimeout = 5 * time.Second
)

// VideoHandler 负责处理视频查询和命令相关的 gRPC 请求。
type VideoHandler struct {
	videov1.UnimplementedVideoQueryServiceServer
	videov1.UnimplementedVideoCommandServiceServer

	uc *services.VideoUsecase
}

// NewVideoHandler 构造视频 Handler。
func NewVideoHandler(uc *services.VideoUsecase) *VideoHandler {
	return &VideoHandler{uc: uc}
}

// CreateVideo 实现 VideoCommandService.CreateVideo RPC。
func (h *VideoHandler) CreateVideo(ctx context.Context, req *videov1.CreateVideoRequest) (*videov1.CreateVideoResponse, error) {
	input, err := mapper.ToCreateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	// 设置创建超时
	timeoutCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	created, err := h.uc.CreateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}

	return views.NewCreateVideoResponse(created), nil
}

// UpdateVideo 实现 VideoCommandService.UpdateVideo RPC。
func (h *VideoHandler) UpdateVideo(ctx context.Context, req *videov1.UpdateVideoRequest) (*videov1.UpdateVideoResponse, error) {
	input, err := mapper.ToUpdateVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	updated, err := h.uc.UpdateVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return views.NewUpdateVideoResponse(updated), nil
}

// DeleteVideo 实现 VideoCommandService.DeleteVideo RPC。
func (h *VideoHandler) DeleteVideo(ctx context.Context, req *videov1.DeleteVideoRequest) (*videov1.DeleteVideoResponse, error) {
	input, err := mapper.ToDeleteVideoInput(req)
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	deleted, err := h.uc.DeleteVideo(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return views.NewDeleteVideoResponse(deleted), nil
}

// GetVideoDetail 实现 VideoQueryService.GetVideoDetail RPC。
func (h *VideoHandler) GetVideoDetail(ctx context.Context, req *videov1.GetVideoDetailRequest) (*videov1.GetVideoDetailResponse, error) {
	videoID, err := mapper.ParseVideoID(req.GetVideoId())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	// 设置查询超时，防止慢查询阻塞
	timeoutCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	detail, err := h.uc.GetVideoDetail(timeoutCtx, videoID)
	if err != nil {
		return nil, err
	}

	return views.NewGetVideoDetailResponse(detail), nil
}
