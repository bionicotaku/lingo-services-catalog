package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUploadNotFound 表示上传会话不存在。
var ErrUploadNotFound = errors.New("upload session not found")

// UploadRepository 封装 catalog.uploads 表的访问逻辑。
type UploadRepository struct {
	db      *pgxpool.Pool
	queries *catalogsql.Queries
	log     *log.Helper
}

// NewUploadRepository 构造 UploadRepository。
func NewUploadRepository(db *pgxpool.Pool, logger log.Logger) *UploadRepository {
	return &UploadRepository{
		db:      db,
		queries: catalogsql.New(db),
		log:     log.NewHelper(logger),
	}
}

// UpsertUploadInput 描述初始化上传会话所需的字段。
type UpsertUploadInput struct {
	VideoID            uuid.UUID
	UserID             uuid.UUID
	Bucket             string
	ObjectName         string
	ContentType        *string
	ExpectedSize       int64
	ContentMD5         string
	Title              string
	Description        string
	SignedURL          *string
	SignedURLExpiresAt *time.Time
}

// Upsert 创建或复用上传会话，返回会话实体及是否新建的标记。
func (r *UploadRepository) Upsert(ctx context.Context, sess txmanager.Session, input UpsertUploadInput) (*po.UploadSession, bool, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := mappers.BuildUpsertUploadParams(
		input.VideoID,
		input.UserID,
		input.Bucket,
		input.ObjectName,
		input.ContentType,
		input.ExpectedSize,
		input.ContentMD5,
		input.Title,
		input.Description,
		input.SignedURL,
		input.SignedURLExpiresAt,
	)

	row, err := queries.UpsertUpload(ctx, params)
	if err != nil {
		r.log.WithContext(ctx).Errorf("upsert upload failed: user_id=%s video_id=%s err=%v", input.UserID, input.VideoID, err)
		return nil, false, fmt.Errorf("upsert upload: %w", err)
	}

	session, inserted := mappers.UploadSessionFromUpsertRow(row)
	return session, inserted, nil
}

// GetByVideoID 查询指定 video_id 的上传会话。
func (r *UploadRepository) GetByVideoID(ctx context.Context, sess txmanager.Session, videoID uuid.UUID) (*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetUploadByVideoID(ctx, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		r.log.WithContext(ctx).Errorf("get upload by video id failed: video_id=%s err=%v", videoID, err)
		return nil, fmt.Errorf("get upload by video id: %w", err)
	}

	return mappers.UploadSessionFromCatalog(record), nil
}

// GetByUserMD5 查询指定用户与内容哈希的上传会话。
func (r *UploadRepository) GetByUserMD5(ctx context.Context, sess txmanager.Session, userID uuid.UUID, contentMD5 string) (*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetUploadByUserMd5(ctx, catalogsql.GetUploadByUserMd5Params{
		UserID:     userID,
		ContentMd5: contentMD5,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		r.log.WithContext(ctx).Errorf("get upload by user/md5 failed: user_id=%s err=%v", userID, err)
		return nil, fmt.Errorf("get upload by user md5: %w", err)
	}

	return mappers.UploadSessionFromCatalog(record), nil
}

// GetByObject 查询指定 bucket/object_name 的上传会话。
func (r *UploadRepository) GetByObject(ctx context.Context, sess txmanager.Session, bucket, objectName string) (*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetUploadByObject(ctx, catalogsql.GetUploadByObjectParams{
		Bucket:     bucket,
		ObjectName: objectName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		r.log.WithContext(ctx).Errorf("get upload by object failed: bucket=%s object=%s err=%v", bucket, objectName, err)
		return nil, fmt.Errorf("get upload by object: %w", err)
	}

	return mappers.UploadSessionFromCatalog(record), nil
}

// MarkCompleted 将上传会话标记为 completed，并回写对象校验信息。
func (r *UploadRepository) MarkCompleted(ctx context.Context, sess txmanager.Session, input MarkUploadCompletedInput) (*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := mappers.BuildMarkUploadCompletedParams(
		input.VideoID,
		input.SizeBytes,
		input.MD5Hash,
		input.CRC32C,
		input.GCSGeneration,
		input.GCSEtag,
		input.ContentType,
	)

	record, err := queries.MarkUploadCompleted(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		r.log.WithContext(ctx).Errorf("mark upload completed failed: video_id=%s err=%v", input.VideoID, err)
		return nil, fmt.Errorf("mark upload completed: %w", err)
	}

	return mappers.UploadSessionFromCatalog(record), nil
}

// MarkUploadCompletedInput 描述回调写入成功时的字段。
type MarkUploadCompletedInput struct {
	VideoID       uuid.UUID
	SizeBytes     int64
	MD5Hash       *string
	CRC32C        *string
	GCSGeneration *string
	GCSEtag       *string
	ContentType   *string
}

// MarkFailed 将上传会话标记为失败并记录原因。
func (r *UploadRepository) MarkFailed(ctx context.Context, sess txmanager.Session, input MarkUploadFailedInput) (*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := mappers.BuildMarkUploadFailedParams(input.VideoID, input.ErrorCode, input.ErrorMessage)

	record, err := queries.MarkUploadFailed(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		r.log.WithContext(ctx).Errorf("mark upload failed: video_id=%s err=%v", input.VideoID, err)
		return nil, fmt.Errorf("mark upload failed: %w", err)
	}

	return mappers.UploadSessionFromCatalog(record), nil
}

// MarkUploadFailedInput 描述写失败时的参数。
type MarkUploadFailedInput struct {
	VideoID      uuid.UUID
	ErrorCode    *string
	ErrorMessage *string
}

// ListExpiredUploads 返回已过期但仍处于 uploading 状态的会话列表。
func (r *UploadRepository) ListExpiredUploads(ctx context.Context, sess txmanager.Session, cutoff time.Time, limit int32) ([]*po.UploadSession, error) {
	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := queries.ListExpiredUploads(ctx, catalogsql.ListExpiredUploadsParams{
		Cutoff: mappers.ToPgTimestamptz(&cutoff),
		Limit:  limit,
	})
	if err != nil {
		r.log.WithContext(ctx).Errorf("list expired uploads failed: cutoff=%s err=%v", cutoff, err)
		return nil, fmt.Errorf("list expired uploads: %w", err)
	}

	sessions := make([]*po.UploadSession, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, mappers.UploadSessionFromCatalog(row))
	}
	return sessions, nil
}
