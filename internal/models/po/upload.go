package po

import (
	"time"

	"github.com/google/uuid"
)

// UploadStatus 表示上传会话的当前状态。
type UploadStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusCompleted UploadStatus = "completed"
	UploadStatusFailed    UploadStatus = "failed"
)

// UploadSession 描述 catalog.uploads 表中的一条上传会话记录。
type UploadSession struct {
	VideoID            uuid.UUID
	UserID             uuid.UUID
	Bucket             string
	ObjectName         string
	ContentType        *string
	ExpectedSize       int64
	SizeBytes          int64
	ContentMD5         string
	Title              string
	Description        string
	SignedURL          *string
	SignedURLExpiresAt *time.Time
	Status             UploadStatus
	GCSGeneration      *string
	GCSEtag            *string
	MD5Hash            *string
	CRC32C             *string
	ErrorCode          *string
	ErrorMessage       *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
