package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"

	"github.com/bionicotaku/lingo-utils/txmanager"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// UploadSigner 定义生成 Resumable Upload 签名 URL 的能力。
type UploadSigner interface {
	SignedResumableInitURL(ctx context.Context, bucket, objectName, contentType string, ttl time.Duration) (string, time.Time, error)
}

// UploadRepositoryContract 抽象上传会话持久化操作，便于测试。
type UploadRepositoryContract interface {
	Upsert(ctx context.Context, sess txmanager.Session, input repositories.UpsertUploadInput) (*po.UploadSession, bool, error)
	GetByUserMD5(ctx context.Context, sess txmanager.Session, userID uuid.UUID, contentMD5 string) (*po.UploadSession, error)
}

// InitResumableUploadInput 为服务层输入。
type InitResumableUploadInput struct {
	UserID          uuid.UUID
	SizeBytes       int64
	ContentType     string
	ContentMD5Hex   string
	DurationSeconds int32
	Title           string
	Description     string
	IdempotencyKey  string
}

// InitResumableUploadResult 为服务层输出。
type InitResumableUploadResult struct {
	Session          *po.UploadSession
	ResumableInitURL string
	ExpiresAt        time.Time
	Reused           bool
}

// UploadService 实现上传会话的业务用例。
type UploadService struct {
	repo        UploadRepositoryContract
	signer      UploadSigner
	bucket      string
	ttl         time.Duration
	log         *log.Helper
	now         func() time.Time
	allowedMIME map[string]struct{}
}

// NewUploadService 创建 UploadService。
func NewUploadService(repo UploadRepositoryContract, signer UploadSigner, bucket string, ttl time.Duration, logger log.Logger) (*UploadService, error) {
	switch {
	case repo == nil:
		return nil, errors.New("upload service: repository is required")
	case signer == nil:
		return nil, errors.New("upload service: signer is required")
	case bucket == "":
		return nil, errors.New("upload service: bucket is required")
	case ttl <= 0:
		return nil, errors.New("upload service: ttl must be positive")
	}

	svc := &UploadService{
		repo:   repo,
		signer: signer,
		bucket: bucket,
		ttl:    ttl,
		now:    time.Now,
		allowedMIME: map[string]struct{}{
			"video/mp4":                {},
			"video/quicktime":          {},
			"video/x-m4v":              {},
			"video/webm":               {},
			"video/3gpp":               {},
			"video/3gpp2":              {},
			"application/octet-stream": {},
		},
		log: log.NewHelper(logger),
	}
	return svc, nil
}

// InitResumableUpload 执行上传会话初始化逻辑。
func (s *UploadService) InitResumableUpload(ctx context.Context, input InitResumableUploadInput) (*InitResumableUploadResult, error) {
	if err := s.validateInput(input); err != nil {
		return nil, err
	}

	md5Hex := strings.ToLower(input.ContentMD5Hex)
	contentType := strings.ToLower(input.ContentType)

	existing, err := s.repo.GetByUserMD5(ctx, nil, input.UserID, md5Hex)
	if err != nil && !errors.Is(err, repositories.ErrUploadNotFound) {
		return nil, fmt.Errorf("lookup upload session: %w", err)
	}

	now := s.now()
	var (
		videoID    uuid.UUID
		objectName string
		signedURL  string
		expiresAt  time.Time
		reused     bool
	)

	if existing != nil {
		if existing.Status == po.UploadStatusCompleted {
			return nil, kerrors.Conflict(videov1.ErrorReason_ERROR_REASON_UPLOAD_ALREADY_COMPLETED.String(), "upload already completed").WithCause(ErrUploadAlreadyCompleted)
		}
		videoID = existing.VideoID
		objectName = existing.ObjectName

		if s.shouldReuseSignedURL(existing, now) {
			if existing.SignedURL != nil {
				signedURL = *existing.SignedURL
			}
			if existing.SignedURLExpiresAt != nil {
				expiresAt = existing.SignedURLExpiresAt.UTC()
			}
			reused = true
		}
	}

	if videoID == uuid.Nil {
		videoID = uuid.New()
		objectName = fmt.Sprintf("raw_videos/%s/%s", input.UserID.String(), videoID.String())
	}

	if signedURL == "" || expiresAt.Before(now.Add(30*time.Second)) {
		signedURL, expiresAt, err = s.signer.SignedResumableInitURL(ctx, s.bucket, objectName, contentType, s.ttl)
		if err != nil {
			return nil, fmt.Errorf("sign resumable init url: %w", err)
		}
	}

	upsertInput := repositories.UpsertUploadInput{
		VideoID:            videoID,
		UserID:             input.UserID,
		Bucket:             s.bucket,
		ObjectName:         objectName,
		ContentType:        nullableString(contentType),
		ExpectedSize:       input.SizeBytes,
		ContentMD5:         md5Hex,
		Title:              input.Title,
		Description:        input.Description,
		SignedURL:          &signedURL,
		SignedURLExpiresAt: &expiresAt,
	}

	session, inserted, err := s.repo.Upsert(ctx, nil, upsertInput)
	if err != nil {
		return nil, fmt.Errorf("persist upload session: %w", err)
	}

	// 以持久化后的数据为准，防止时区差异。
	if session.SignedURL != nil {
		signedURL = *session.SignedURL
	}
	if session.SignedURLExpiresAt != nil {
		expiresAt = session.SignedURLExpiresAt.UTC()
	}
	if !inserted {
		reused = true
	}

	return &InitResumableUploadResult{
		Session:          session,
		ResumableInitURL: signedURL,
		ExpiresAt:        expiresAt,
		Reused:           reused,
	}, nil
}

// ErrUploadAlreadyCompleted 在重复上传时返回。
var ErrUploadAlreadyCompleted = errors.New("upload already completed")

func (s *UploadService) validateInput(input InitResumableUploadInput) error {
	if input.UserID == uuid.Nil {
		return kerrors.Unauthorized(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "user metadata is required")
	}
	if input.ContentMD5Hex == "" {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "content_md5_hex is required")
	}
	if len(input.ContentMD5Hex) != 32 {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "content_md5_hex must be 32 hex characters")
	}
	if input.Title == "" {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "title is required")
	}
	if input.Description == "" {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "description is required")
	}
	if input.DurationSeconds <= 0 || input.DurationSeconds > 300 {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "duration_seconds must be within 1-300 seconds")
	}
	if input.SizeBytes < 0 {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), "size_bytes must be non-negative")
	}
	if _, ok := s.allowedMIME[strings.ToLower(input.ContentType)]; !ok {
		return kerrors.BadRequest(videov1.ErrorReason_ERROR_REASON_UPLOAD_INVALID.String(), fmt.Sprintf("unsupported content_type: %s", input.ContentType))
	}
	return nil
}

func (s *UploadService) shouldReuseSignedURL(session *po.UploadSession, now time.Time) bool {
	if session == nil {
		return false
	}
	if session.Status != po.UploadStatusUploading {
		return false
	}
	if session.SignedURL == nil || session.SignedURLExpiresAt == nil {
		return false
	}
	return session.SignedURLExpiresAt.After(now.Add(30 * time.Second))
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	trimmed := value
	return &trimmed
}
