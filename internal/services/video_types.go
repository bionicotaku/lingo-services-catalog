package services

import (
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/go-kratos/kratos/v2/errors"
)

// ErrVideoNotFound 是当视频未找到时返回的哨兵错误。
var ErrVideoNotFound = errors.NotFound(videov1.ErrorReason_ERROR_REASON_VIDEO_NOT_FOUND.String(), "video not found")
