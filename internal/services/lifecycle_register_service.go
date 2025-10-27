package services

import (
	"context"

	"github.com/google/uuid"
)

// RegisterUploadInput 描述注册上传时所需的最小字段。
type RegisterUploadInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
	IdempotencyKey   string
}

// RegisterUploadService 封装上传注册用例，复用现有写模型实现。
type RegisterUploadService struct {
	writer *LifecycleWriter
}

// NewRegisterUploadService 构造上传注册服务。
func NewRegisterUploadService(writer *LifecycleWriter) *RegisterUploadService {
	return &RegisterUploadService{writer: writer}
}

// RegisterUpload 创建视频基础记录，并写入 Outbox 事件。
func (s *RegisterUploadService) RegisterUpload(ctx context.Context, input RegisterUploadInput) (*VideoRevision, error) {
	return s.writer.CreateVideo(ctx, CreateVideoInput(input))
}
