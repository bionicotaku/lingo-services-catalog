package uploads

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-utils/outbox/store"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const (
	gcsObjectFinalizeEvent = "OBJECT_FINALIZE"

	errorCodeMD5Mismatch = "MD5_MISMATCH"
)

type uploadRepository interface {
	GetByObject(ctx context.Context, sess txmanager.Session, bucket, objectName string) (*po.UploadSession, error)
	MarkCompleted(ctx context.Context, sess txmanager.Session, input repositories.MarkUploadCompletedInput) (*po.UploadSession, error)
	MarkFailed(ctx context.Context, sess txmanager.Session, input repositories.MarkUploadFailedInput) (*po.UploadSession, error)
}

// Handler 处理上传完成事件，将对象落地到视频主表并触发领域事件。
type Handler struct {
	uploads uploadRepository
	writer  *services.LifecycleWriter
	log     *log.Helper
}

// NewHandler 构造上传事件处理器。
func NewHandler(repo uploadRepository, writer *services.LifecycleWriter, logger log.Logger) *Handler {
	if logger == nil {
		logger = log.NewStdLogger(nil)
	}
	return &Handler{
		uploads: repo,
		writer:  writer,
		log:     log.NewHelper(logger),
	}
}

// Handle 执行 OBJECT_FINALIZE 事件的业务处理。
func (h *Handler) Handle(ctx context.Context, sess txmanager.Session, evt *Event, inboxEvt *store.InboxEvent) error {
	if evt == nil {
		return fmt.Errorf("uploads: nil event payload")
	}
	if inboxEvt == nil {
		return fmt.Errorf("uploads: missing inbox event metadata")
	}
	if !strings.EqualFold(inboxEvt.EventType, gcsObjectFinalizeEvent) {
		return nil
	}
	if h.uploads == nil || h.writer == nil {
		return fmt.Errorf("uploads: handler not initialized")
	}

	session, err := h.uploads.GetByObject(ctx, sess, evt.Bucket, evt.ObjectName)
	if err != nil {
		if errors.Is(err, repositories.ErrUploadNotFound) {
			h.log.WithContext(ctx).Warnf("uploads: finalize for unknown object bucket=%s object=%s", evt.Bucket, evt.ObjectName)
			return nil
		}
		return fmt.Errorf("uploads: load session: %w", err)
	}

	if session.Status == po.UploadStatusCompleted {
		if session.GCSGeneration != nil && evt.Generation != "" && strings.EqualFold(*session.GCSGeneration, evt.Generation) {
			h.log.WithContext(ctx).Debugf("uploads: skip duplicate finalize bucket=%s object=%s generation=%s", evt.Bucket, evt.ObjectName, evt.Generation)
			return nil
		}
	}

	md5Hex, err := base64MD5ToHex(evt.MD5Base64)
	if err != nil {
		return fmt.Errorf("uploads: decode md5: %w", err)
	}
	expectedMD5 := strings.ToLower(session.ContentMD5)
	if expectedMD5 != "" && md5Hex != "" && md5Hex != expectedMD5 {
		h.log.WithContext(ctx).Warnf("uploads: md5 mismatch video_id=%s expected=%s actual=%s", session.VideoID, expectedMD5, md5Hex)
		if _, failErr := h.uploads.MarkFailed(ctx, sess, repositories.MarkUploadFailedInput{
			VideoID:      session.VideoID,
			ErrorCode:    strPtr(errorCodeMD5Mismatch),
			ErrorMessage: strPtr(fmt.Sprintf("md5 mismatch: expected %s actual %s", expectedMD5, md5Hex)),
		}); failErr != nil {
			return fmt.Errorf("uploads: mark failed: %w", failErr)
		}
		return nil
	}

	completed, err := h.uploads.MarkCompleted(ctx, sess, repositories.MarkUploadCompletedInput{
		VideoID:       session.VideoID,
		SizeBytes:     evt.SizeBytes,
		MD5Hash:       optionalString(md5Hex),
		CRC32C:        optionalString(evt.CRC32C),
		GCSGeneration: optionalString(evt.Generation),
		GCSEtag:       optionalString(evt.ETag),
		ContentType:   optionalString(evt.ContentType),
	})
	if err != nil {
		return fmt.Errorf("uploads: mark completed: %w", err)
	}

	if session.Status == po.UploadStatusCompleted && completed.Status == po.UploadStatusCompleted {
		// 已在前序处理完成，无需重复创建视频。
		return nil
	}

	rawReference := fmt.Sprintf("gs://%s/%s", evt.Bucket, evt.ObjectName)
	createInput := services.CreateVideoInput{
		VideoID:          session.VideoID,
		UploadUserID:     session.UserID,
		Title:            session.Title,
		Description:      stringPtrNonEmpty(session.Description),
		RawFileReference: rawReference,
	}
	revision, err := h.writer.CreateVideo(ctx, createInput)
	if err != nil {
		return fmt.Errorf("uploads: create video: %w", err)
	}

	var (
		statusProcessing = po.VideoStatusProcessing
		update           services.UpdateVideoInput
	)
	update.VideoID = session.VideoID

	if revision != nil && revision.EventID != uuid.Nil && revision.Status == po.VideoStatusPendingUpload {
		update.Status = &statusProcessing
	}
	if completed.SizeBytes > 0 {
		size := completed.SizeBytes
		update.RawFileSize = &size
	}

	if update.Status != nil || update.RawFileSize != nil {
		if _, err := h.writer.UpdateVideo(ctx, update); err != nil {
			return fmt.Errorf("uploads: update video: %w", err)
		}
	}

	h.log.WithContext(ctx).Infof("uploads: finalized video_id=%s object=%s generation=%s size=%d", session.VideoID, evt.ObjectName, evt.Generation, evt.SizeBytes)
	return nil
}

func base64MD5ToHex(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(decoded)), nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v := value
	return &v
}

func stringPtrNonEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v := value
	return &v
}

func strPtr(value string) *string {
	return &value
}
