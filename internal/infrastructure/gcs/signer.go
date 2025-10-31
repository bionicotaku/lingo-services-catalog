// Package gcs 提供与 Google Cloud Storage 交互的基础设施封装。
package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/oauth2/google"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
)

// ResumableSigner 负责生成用于初始化 Resumable Upload 的 V4 Signed URL。
type ResumableSigner struct {
	googleAccessID string
	privateKey     []byte
	now            func() time.Time
	log            *log.Helper
}

// Option 定义可选配置。
type Option func(*ResumableSigner)

// WithClock 覆盖时间获取函数，便于测试。
func WithClock(clock func() time.Time) Option {
	return func(s *ResumableSigner) {
		if clock != nil {
			s.now = clock
		}
	}
}

// WithServiceAccountKey 允许直接注入访问 ID 与私钥（测试友好）。
func WithServiceAccountKey(accessID string, privateKey []byte) Option {
	return func(s *ResumableSigner) {
		if accessID != "" {
			s.googleAccessID = accessID
		}
		if len(privateKey) > 0 {
			s.privateKey = append([]byte(nil), privateKey...)
		}
	}
}

// NewResumableSigner 创建 ResumableSigner，要求默认凭据中包含 service account 私钥。
func NewResumableSigner(ctx context.Context, accessID string, logger log.Logger, opts ...Option) (*ResumableSigner, error) {
	signer := &ResumableSigner{
		googleAccessID: accessID,
		now:            time.Now,
		log:            log.NewHelper(logger),
	}

	for _, opt := range opts {
		opt(signer)
	}

	if len(signer.privateKey) == 0 {
		privKey, detectedAccessID, err := loadServiceAccountKey(ctx)
		if err != nil {
			return nil, fmt.Errorf("init gcs signer: %w", err)
		}
		signer.privateKey = privKey
		if signer.googleAccessID == "" {
			signer.googleAccessID = detectedAccessID
		} else if detectedAccessID != "" && detectedAccessID != signer.googleAccessID {
			signer.log.WithContext(ctx).Warnf("gcs signer access id mismatch: config=%s credentials=%s", signer.googleAccessID, detectedAccessID)
		}
	}

	if signer.googleAccessID == "" {
		return nil, errors.New("gcs signer: google access id is required")
	}
	if len(signer.privateKey) == 0 {
		return nil, errors.New("gcs signer: private key is required")
	}

	return signer, nil
}

// SignedResumableInitURL 生成 Resumable Upload 初始化所需的 Signed URL。
func (s *ResumableSigner) SignedResumableInitURL(ctx context.Context, bucket, objectName, contentType string, ttl time.Duration) (signedURL string, expires time.Time, err error) {
	if bucket == "" {
		return "", time.Time{}, errors.New("bucket is required")
	}
	if objectName == "" {
		return "", time.Time{}, errors.New("object name is required")
	}
	if ttl <= 0 {
		return "", time.Time{}, errors.New("ttl must be positive")
	}

	expires = s.now().Add(ttl)
	headers := []string{"x-goog-resumable:start", "x-goog-if-generation-match:0"}
	if contentType != "" {
		headers = append(headers, "x-upload-content-type:"+contentType)
	}

	opts := &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         http.MethodPost,
		Expires:        expires,
		Headers:        headers,
		GoogleAccessID: s.googleAccessID,
		PrivateKey:     s.privateKey,
	}

	url, signErr := storage.SignedURL(bucket, objectName, opts)
	if signErr != nil {
		s.log.WithContext(ctx).Errorf("generate resumable signed url failed: bucket=%s object=%s err=%v", bucket, objectName, signErr)
		return "", time.Time{}, fmt.Errorf("signed url: %w", signErr)
	}
	return url, expires, nil
}

type serviceAccountKey struct {
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
}

func loadServiceAccountKey(ctx context.Context) ([]byte, string, error) {
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("find default credentials: %w", err)
	}
	if len(creds.JSON) == 0 {
		return nil, "", errors.New("service account JSON not found in default credentials")
	}

	var key serviceAccountKey
	if err := json.Unmarshal(creds.JSON, &key); err != nil {
		return nil, "", fmt.Errorf("parse service account json: %w", err)
	}
	if key.PrivateKey == "" {
		return nil, "", errors.New("service account private key is empty; use a service account JSON credential")
	}
	return []byte(key.PrivateKey), key.ClientEmail, nil
}

// ProvideResumableSigner 供 Wire 注入使用。
func ProvideResumableSigner(ctx context.Context, cfg configloader.GCSConfig, logger log.Logger) (*ResumableSigner, error) {
	signer, err := NewResumableSigner(ctx, cfg.SignerServiceAccount, logger)
	if err != nil {
		return nil, err
	}
	return signer, nil
}
