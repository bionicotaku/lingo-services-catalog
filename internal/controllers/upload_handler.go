package controllers

import (
	"context"
	"strings"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers/dto"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// UploadHandler 实现 UploadService gRPC 接口。
type UploadHandler struct {
	videov1.UnimplementedUploadServiceServer

	*BaseHandler
	svc *services.UploadService
}

// NewUploadHandler 构造 UploadHandler。
func NewUploadHandler(base *BaseHandler, svc *services.UploadService) *UploadHandler {
	if base == nil {
		base = NewBaseHandler(HandlerTimeouts{})
	}
	return &UploadHandler{BaseHandler: base, svc: svc}
}

// InitResumableUpload 处理上传会话初始化请求。
func (h *UploadHandler) InitResumableUpload(ctx context.Context, req *videov1.InitResumableUploadRequest) (*videov1.InitResumableUploadResponse, error) {
	if h.svc == nil {
		return nil, kerrors.InternalServer(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "upload service not available")
	}
	meta := h.ExtractMetadata(ctx)
	if meta.InvalidUserInfo {
		return nil, kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "invalid user metadata")
	}
	userID, ok := meta.UserUUID()
	if !ok {
		if strings.TrimSpace(meta.UserID) != "" || strings.TrimSpace(meta.RawUserInfo) != "" {
			return nil, kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "invalid user metadata")
		}
		return nil, kerrors.Unauthorized(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "user metadata required")
	}

	input := dto.ToInitResumableUploadInput(req, userID, meta)

	timeoutCtx, cancel := h.WithTimeout(ctx, HandlerTypeCommand)
	defer cancel()
	timeoutCtx = InjectHandlerMetadata(timeoutCtx, meta)

	result, err := h.svc.InitResumableUpload(timeoutCtx, input)
	if err != nil {
		if ke := kerrors.FromError(err); ke != nil {
			return nil, ke
		}
		return nil, kerrors.InternalServer(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "init upload failed").WithCause(err)
	}
	if result == nil || result.Session == nil {
		return nil, kerrors.InternalServer(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "empty upload result")
	}

	resp := &videov1.InitResumableUploadResponse{
		VideoId:          result.Session.VideoID.String(),
		ResumableInitUrl: result.ResumableInitURL,
		ExpiresAtUnixms:  result.ExpiresAt.UTC().UnixMilli(),
	}
	return resp, nil
}
