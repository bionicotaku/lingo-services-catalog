package controllers

import (
	"context"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers/dto"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/go-kratos/kratos/v2/errors"
)

// VideoQueryHandler 负责处理视频查询相关的 gRPC 请求。
type VideoQueryHandler struct {
	videov1.UnimplementedCatalogQueryServiceServer

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

// GetVideoMetadata 返回独立的媒体/AI 元数据。
func (h *VideoQueryHandler) GetVideoMetadata(ctx context.Context, req *videov1.GetVideoMetadataRequest) (*videov1.GetVideoMetadataResponse, error) {
	videoID, err := dto.ParseVideoID(req.GetVideoId())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_ID_INVALID.String(), err.Error())
	}

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeQuery)
	defer cancel()

	meta, err := h.svc.GetVideoMetadata(timeoutCtx, videoID)
	if err != nil {
		return nil, err
	}
	return dto.NewGetVideoMetadataResponse(meta), nil
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

	detail, metadata, err := h.svc.GetVideoDetail(timeoutCtx, videoID)
	if err != nil {
		return nil, err
	}
	return dto.NewGetVideoDetailResponse(detail, metadata), nil
}

// ListUserPublicVideos 实现公共视频列表查询。
func (h *VideoQueryHandler) ListUserPublicVideos(ctx context.Context, req *videov1.ListUserPublicVideosRequest) (*videov1.ListUserPublicVideosResponse, error) {
	meta := h.ExtractMetadata(ctx)
	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeQuery)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	items, nextToken, err := h.svc.ListUserPublicVideos(timeoutCtx, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &videov1.ListUserPublicVideosResponse{
		Videos:        dto.NewVideoListItems(items),
		NextPageToken: nextToken,
	}, nil
}

// ListMyUploads 实现用户上传列表查询。
func (h *VideoQueryHandler) ListMyUploads(ctx context.Context, req *videov1.ListMyUploadsRequest) (*videov1.ListMyUploadsResponse, error) {
	meta := h.ExtractMetadata(ctx)

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeQuery)
	defer cancel()

	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	statuses, err := dto.ParseStatusFilters(req.GetStatusFilter())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), err.Error())
	}
	stages, err := dto.ParseStageFilters(req.GetStageFilter())
	if err != nil {
		return nil, errors.BadRequest(videov1.ErrorReason_ERROR_REASON_VIDEO_UPDATE_INVALID.String(), err.Error())
	}

	items, nextToken, svcErr := h.svc.ListMyUploads(timeoutCtx, req.GetPageSize(), req.GetPageToken(), statuses, stages)
	if svcErr != nil {
		return nil, svcErr
	}
	return &videov1.ListMyUploadsResponse{
		Videos:        dto.NewMyUploadListItems(items),
		NextPageToken: nextToken,
	}, nil
}
