package controllers

import (
	"context"
	"fmt"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/internal/views"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// VideoHandler 负责处理视频查询相关的 gRPC 请求。
type VideoHandler struct {
	videov1.UnimplementedVideoQueryServiceServer

	uc *services.VideoUsecase
}

// NewVideoHandler 构造视频查询 Handler。
func NewVideoHandler(uc *services.VideoUsecase) *VideoHandler {
	return &VideoHandler{uc: uc}
}

// GetVideoDetail 实现 VideoQueryService.GetVideoDetail RPC。
func (h *VideoHandler) GetVideoDetail(ctx context.Context, req *videov1.GetVideoDetailRequest) (*videov1.GetVideoDetailResponse, error) {
	if req.GetVideoId() == "" {
		return nil, errors.BadRequest("VIDEO_ID_REQUIRED", "video_id is required")
	}

	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return nil, errors.BadRequest("VIDEO_ID_INVALID", fmt.Sprintf("invalid video_id: %v", err))
	}

	detail, err := h.uc.GetVideoDetail(ctx, videoID)
	if err != nil {
		return nil, err
	}

	return views.NewGetVideoDetailResponse(detail), nil
}
