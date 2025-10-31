package mappers

import (
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"

	"github.com/google/uuid"
)

// UploadSessionFromCatalog 将 sqlc 生成的 CatalogUpload 转换为领域实体。
func UploadSessionFromCatalog(row catalogsql.CatalogUpload) *po.UploadSession {
	return &po.UploadSession{
		VideoID:            row.VideoID,
		UserID:             row.UserID,
		Bucket:             row.Bucket,
		ObjectName:         row.ObjectName,
		ContentType:        textPtr(row.ContentType),
		ExpectedSize:       row.ExpectedSize,
		SizeBytes:          row.SizeBytes,
		ContentMD5:         row.ContentMd5,
		Title:              row.Title,
		Description:        row.Description,
		SignedURL:          textPtr(row.SignedUrl),
		SignedURLExpiresAt: timestampPtr(row.SignedUrlExpiresAt),
		Status:             po.UploadStatus(row.Status),
		GCSGeneration:      textPtr(row.GcsGeneration),
		GCSEtag:            textPtr(row.GcsEtag),
		MD5Hash:            textPtr(row.Md5Hash),
		CRC32C:             textPtr(row.Crc32c),
		ErrorCode:          textPtr(row.ErrorCode),
		ErrorMessage:       textPtr(row.ErrorMessage),
		CreatedAt:          mustTimestamp(row.CreatedAt),
		UpdatedAt:          mustTimestamp(row.UpdatedAt),
	}
}

// UploadSessionFromUpsertRow 将 UpsertUploadRow 转换为领域实体并返回 inserted 标记。
func UploadSessionFromUpsertRow(row catalogsql.UpsertUploadRow) (*po.UploadSession, bool) {
	session := &po.UploadSession{
		VideoID:            row.VideoID,
		UserID:             row.UserID,
		Bucket:             row.Bucket,
		ObjectName:         row.ObjectName,
		ContentType:        textPtr(row.ContentType),
		ExpectedSize:       row.ExpectedSize,
		SizeBytes:          row.SizeBytes,
		ContentMD5:         row.ContentMd5,
		Title:              row.Title,
		Description:        row.Description,
		SignedURL:          textPtr(row.SignedUrl),
		SignedURLExpiresAt: timestampPtr(row.SignedUrlExpiresAt),
		Status:             po.UploadStatus(row.Status),
		GCSGeneration:      textPtr(row.GcsGeneration),
		GCSEtag:            textPtr(row.GcsEtag),
		MD5Hash:            textPtr(row.Md5Hash),
		CRC32C:             textPtr(row.Crc32c),
		ErrorCode:          textPtr(row.ErrorCode),
		ErrorMessage:       textPtr(row.ErrorMessage),
		CreatedAt:          mustTimestamp(row.CreatedAt),
		UpdatedAt:          mustTimestamp(row.UpdatedAt),
	}
	return session, row.Inserted
}

// BuildUpsertUploadParams 构造 UpsertUpload 的 sqlc 参数。
func BuildUpsertUploadParams(
	videoID uuid.UUID,
	userID uuid.UUID,
	bucket string,
	objectName string,
	contentType *string,
	expectedSize int64,
	contentMD5 string,
	title string,
	description string,
	signedURL *string,
	signedURLExpiresAt *time.Time,
) catalogsql.UpsertUploadParams {
	return catalogsql.UpsertUploadParams{
		VideoID:            videoID,
		UserID:             userID,
		Bucket:             bucket,
		ObjectName:         objectName,
		ContentType:        ToPgText(contentType),
		ExpectedSize:       expectedSize,
		ContentMd5:         contentMD5,
		Title:              title,
		Description:        description,
		SignedUrl:          ToPgText(signedURL),
		SignedUrlExpiresAt: ToPgTimestamptz(signedURLExpiresAt),
		Status:             string(po.UploadStatusUploading),
	}
}

// BuildMarkUploadCompletedParams 构造 MarkUploadCompleted 的参数。
func BuildMarkUploadCompletedParams(
	videoID uuid.UUID,
	sizeBytes int64,
	md5Hash *string,
	crc32c *string,
	gcsGeneration *string,
	gcsEtag *string,
	contentType *string,
) catalogsql.MarkUploadCompletedParams {
	return catalogsql.MarkUploadCompletedParams{
		SizeBytes:     sizeBytes,
		Md5Hash:       ToPgText(md5Hash),
		Crc32c:        ToPgText(crc32c),
		GcsGeneration: ToPgText(gcsGeneration),
		GcsEtag:       ToPgText(gcsEtag),
		ContentType:   ToPgText(contentType),
		VideoID:       videoID,
	}
}

// BuildMarkUploadFailedParams 构造 MarkUploadFailed 的参数。
func BuildMarkUploadFailedParams(videoID uuid.UUID, errorCode, errorMessage *string) catalogsql.MarkUploadFailedParams {
	return catalogsql.MarkUploadFailedParams{
		ErrorCode:    ToPgText(errorCode),
		ErrorMessage: ToPgText(errorMessage),
		VideoID:      videoID,
	}
}
