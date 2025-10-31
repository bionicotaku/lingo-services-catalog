package dto

import (
	"strings"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/metadata"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/google/uuid"
)

// ToInitResumableUploadInput 将 gRPC 请求与元数据转换为服务层输入。
func ToInitResumableUploadInput(req *videov1.InitResumableUploadRequest, userID uuid.UUID, meta metadata.HandlerMetadata) services.InitResumableUploadInput {
	if req == nil {
		return services.InitResumableUploadInput{}
	}
	return services.InitResumableUploadInput{
		UserID:          userID,
		SizeBytes:       req.GetSizeBytes(),
		ContentType:     strings.TrimSpace(req.GetContentType()),
		ContentMD5Hex:   strings.TrimSpace(strings.ToLower(req.GetContentMd5Hex())),
		DurationSeconds: req.GetDurationSeconds(),
		Title:           strings.TrimSpace(req.GetTitle()),
		Description:     strings.TrimSpace(req.GetDescription()),
		IdempotencyKey:  strings.TrimSpace(meta.IdempotencyKey),
	}
}
