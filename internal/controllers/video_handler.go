package controllers

import (
	"context"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/internal/views"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
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
	// 参数校验
	if req.GetUploadUserId() == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "upload_user_id is required")
	}
	if req.GetTitle() == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "title is required")
	}
	if req.GetRawFileReference() == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "raw_file_reference is required")
	}

	uploaderID, err := uuid.Parse(req.GetUploadUserId())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), fmt.Sprintf("invalid upload_user_id: %v", err))
	}

	// 构造 Service 输入参数
	input := services.CreateVideoInput{
		UploadUserID:     uploaderID,
		Title:            req.GetTitle(),
		RawFileReference: req.GetRawFileReference(),
	}

	// 处理可选的 description 字段
	if req.GetDescription() != nil {
		desc := req.GetDescription().Value
		input.Description = &desc
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

// GetVideoDetail 实现 VideoQueryService.GetVideoDetail RPC。
func (h *VideoHandler) GetVideoDetail(ctx context.Context, req *videov1.GetVideoDetailRequest) (*videov1.GetVideoDetailResponse, error) {
	if req.GetVideoId() == "" {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), "video_id is required")
	}

	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), fmt.Sprintf("invalid video_id: %v", err))
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
