package controllers

import (
	"context"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers/dto"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/errors"
)

// VideoQueryHandler 负责处理视频查询相关的 gRPC 请求。
type VideoQueryHandler struct {
	videov1.UnimplementedVideoQueryServiceServer

	*BaseHandler
	svc *services.VideoQueryService
}

// NewVideoQueryHandler 构造查询 Handler。
func NewVideoQueryHandler(svc *services.VideoQueryService, base *BaseHandler) *VideoQueryHandler {
	if base == nil {
		base = NewBaseHandler(HandlerTimeouts{})
	}
	return &VideoQueryHandler{BaseHandler: base, svc: svc}
}

// GetVideoDetail 实现 VideoQueryService.GetVideoDetail RPC。
func (h *VideoQueryHandler) GetVideoDetail(ctx context.Context, req *videov1.GetVideoDetailRequest) (*videov1.GetVideoDetailResponse, error) {
	videoID, err := dto.ParseVideoID(req.GetVideoId())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	meta := h.ExtractMetadata(ctx)
	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeQuery)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	detail, err := h.svc.GetVideoDetail(timeoutCtx, videoID)
	if err != nil {
		return nil, err
	}
	return dto.NewGetVideoDetailResponse(detail), nil
}
