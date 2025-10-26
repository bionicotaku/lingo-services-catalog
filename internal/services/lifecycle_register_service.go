package services

import (
	"context"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
	"github.com/google/uuid"
)

// RegisterUploadInput 描述注册上传时所需的最小字段。
type RegisterUploadInput struct {
	UploadUserID     uuid.UUID
	Title            string
	Description      *string
	RawFileReference string
}

// RegisterUploadService 封装上传注册用例，复用现有写模型实现。
type RegisterUploadService struct {
	commands *VideoCommandService
}

// NewRegisterUploadService 构造上传注册服务。
func NewRegisterUploadService(commands *VideoCommandService) *RegisterUploadService {
	return &RegisterUploadService{commands: commands}
}

// RegisterUpload 创建视频基础记录，并写入 Outbox 事件。
func (s *RegisterUploadService) RegisterUpload(ctx context.Context, input RegisterUploadInput) (*vo.VideoCreated, error) {
	createInput := CreateVideoInput{
		UploadUserID:     input.UploadUserID,
		Title:            input.Title,
		Description:      input.Description,
		RawFileReference: input.RawFileReference,
	}
	return s.commands.CreateVideo(ctx, createInput)
}
