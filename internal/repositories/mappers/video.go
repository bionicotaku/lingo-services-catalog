// Package mappers 提供仓储层的模型转换工具，将存储层结果映射为领域实体。
package mappers

import (
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// BuildCreateVideoParams 将仓储层输入转换为 sqlc CreateVideoParams，统一处理可空字段。
func BuildCreateVideoParams(uploadUserID uuid.UUID, title, rawFileReference string, description, visibilityStatus *string, publishAt *time.Time) catalogsql.CreateVideoParams {
	return catalogsql.CreateVideoParams{
		UploadUserID:     uploadUserID,
		Title:            title,
		RawFileReference: rawFileReference,
		Description:      textFromPtr(description),
		VisibilityStatus: ToPgText(visibilityStatus),
		PublishAt:        ToPgTimestamptz(publishAt),
	}
}

// BuildUpdateVideoParams 将更新输入转换为 sqlc UpdateVideoParams。
func BuildUpdateVideoParams(
	videoID uuid.UUID,
	title, description, thumbnailURL, hlsMasterPlaylist, difficulty, summary, rawSubtitleURL, errorMessage *string,
	status *po.VideoStatus,
	mediaStatus, analysisStatus *po.StageStatus,
	rawFileSize *int64,
	rawResolution *string,
	rawBitrate *int32,
	durationMicros *int64,
	encodedResolution *string,
	encodedBitrate *int32,
	mediaJobID, analysisJobID *string,
	mediaEmittedAt, analysisEmittedAt *time.Time,
	tags []string,
	visibilityStatus *string,
	publishAt *time.Time,
) catalogsql.UpdateVideoParams {
	return catalogsql.UpdateVideoParams{
		Title:             ToPgText(title),
		Description:       ToPgText(description),
		Status:            ToNullVideoStatus(status),
		MediaStatus:       ToNullStageStatus(mediaStatus),
		AnalysisStatus:    ToNullStageStatus(analysisStatus),
		RawFileSize:       ToPgInt8(rawFileSize),
		RawResolution:     ToPgText(rawResolution),
		RawBitrate:        ToPgInt4(rawBitrate),
		DurationMicros:    ToPgInt8(durationMicros),
		EncodedResolution: ToPgText(encodedResolution),
		EncodedBitrate:    ToPgInt4(encodedBitrate),
		ThumbnailUrl:      ToPgText(thumbnailURL),
		HlsMasterPlaylist: ToPgText(hlsMasterPlaylist),
		Difficulty:        ToPgText(difficulty),
		Summary:           ToPgText(summary),
		Tags:              append([]string(nil), tags...),
		VisibilityStatus:  ToPgText(visibilityStatus),
		PublishAt:         ToPgTimestamptz(publishAt),
		RawSubtitleUrl:    ToPgText(rawSubtitleURL),
		ErrorMessage:      ToPgText(errorMessage),
		MediaJobID:        ToPgText(mediaJobID),
		MediaEmittedAt:    ToPgTimestamptz(mediaEmittedAt),
		AnalysisJobID:     ToPgText(analysisJobID),
		AnalysisEmittedAt: ToPgTimestamptz(analysisEmittedAt),
		VideoID:           videoID,
	}
}

// VideoFromCatalog 将 sqlc 生成的 CatalogVideo 转换为领域实体 po.Video。
func VideoFromCatalog(v catalogsql.CatalogVideo) *po.Video {
	return &po.Video{
		VideoID:           v.VideoID,
		UploadUserID:      v.UploadUserID,
		CreatedAt:         mustTimestamp(v.CreatedAt),
		UpdatedAt:         mustTimestamp(v.UpdatedAt),
		Title:             v.Title,
		Description:       textPtr(v.Description),
		RawFileReference:  v.RawFileReference,
		Status:            po.VideoStatus(v.Status),
		Version:           v.Version,
		MediaStatus:       po.StageStatus(v.MediaStatus),
		AnalysisStatus:    po.StageStatus(v.AnalysisStatus),
		MediaJobID:        textPtr(v.MediaJobID),
		MediaEmittedAt:    timestampPtr(v.MediaEmittedAt),
		AnalysisJobID:     textPtr(v.AnalysisJobID),
		AnalysisEmittedAt: timestampPtr(v.AnalysisEmittedAt),
		RawFileSize:       int8Ptr(v.RawFileSize),
		RawResolution:     textPtr(v.RawResolution),
		RawBitrate:        int4Ptr(v.RawBitrate),
		DurationMicros:    int8Ptr(v.DurationMicros),
		EncodedResolution: textPtr(v.EncodedResolution),
		EncodedBitrate:    int4Ptr(v.EncodedBitrate),
		ThumbnailURL:      textPtr(v.ThumbnailUrl),
		HLSMasterPlaylist: textPtr(v.HlsMasterPlaylist),
		Difficulty:        textPtr(v.Difficulty),
		Summary:           textPtr(v.Summary),
		Tags:              append([]string(nil), v.Tags...),
		VisibilityStatus:  v.VisibilityStatus,
		PublishAt:         timestampPtr(v.PublishAt),
		RawSubtitleURL:    textPtr(v.RawSubtitleUrl),
		ErrorMessage:      textPtr(v.ErrorMessage),
	}
}

// VideoMetadataFromRow 将 sqlc GetVideoMetadataRow 转换为元数据实体。
func VideoMetadataFromRow(row catalogsql.GetVideoMetadataRow) *po.VideoMetadata {
	return &po.VideoMetadata{
		VideoID:           row.VideoID,
		Status:            row.Status,
		MediaStatus:       row.MediaStatus,
		AnalysisStatus:    row.AnalysisStatus,
		DurationMicros:    int8Ptr(row.DurationMicros),
		EncodedResolution: textPtr(row.EncodedResolution),
		EncodedBitrate:    int4Ptr(row.EncodedBitrate),
		ThumbnailURL:      textPtr(row.ThumbnailUrl),
		HLSMasterPlaylist: textPtr(row.HlsMasterPlaylist),
		Difficulty:        textPtr(row.Difficulty),
		Summary:           textPtr(row.Summary),
		Tags:              append([]string(nil), row.Tags...),
		VisibilityStatus:  row.VisibilityStatus,
		PublishAt:         timestampPtr(row.PublishAt),
		RawSubtitleURL:    textPtr(row.RawSubtitleUrl),
		UpdatedAt:         mustTimestamp(row.UpdatedAt),
		Version:           row.Version,
	}
}

// VideoReadyViewFromFindRow 将 FindPublishedVideo 查询结果转换为 po.VideoReadyView。
func VideoReadyViewFromFindRow(v catalogsql.FindPublishedVideoRow) *po.VideoReadyView {
	return &po.VideoReadyView{
		VideoID:          v.VideoID,
		Title:            v.Title,
		Status:           po.VideoStatus(v.Status),
		MediaStatus:      po.StageStatus(v.MediaStatus),
		AnalysisStatus:   po.StageStatus(v.AnalysisStatus),
		CreatedAt:        mustTimestamp(v.CreatedAt),
		UpdatedAt:        mustTimestamp(v.UpdatedAt),
		VisibilityStatus: v.VisibilityStatus,
		PublishAt:        timestampPtr(v.PublishAt),
	}
}

func mustTimestamp(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

func timestampPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int8Ptr(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func int4Ptr(i pgtype.Int4) *int32 {
	if !i.Valid {
		return nil
	}
	return &i.Int32
}

func textFromPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{
		String: *value,
		Valid:  true,
	}
}

// ToPgText 将 string 指针转换为 pgtype.Text。
func ToPgText(value *string) pgtype.Text {
	return textFromPtr(value)
}

// ToPgInt8 将 int64 指针转换为 pgtype.Int8。
func ToPgInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{
		Int64: *value,
		Valid: true,
	}
}

// ToPgInt4 将 int32 指针转换为 pgtype.Int4。
func ToPgInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{
		Int32: *value,
		Valid: true,
	}
}

// ToNullVideoStatus 将领域视频状态转换为 sqlc NullCatalogVideoStatus。
func ToNullVideoStatus(value *po.VideoStatus) catalogsql.NullCatalogVideoStatus {
	if value == nil {
		return catalogsql.NullCatalogVideoStatus{}
	}
	return catalogsql.NullCatalogVideoStatus{
		CatalogVideoStatus: catalogsql.CatalogVideoStatus(*value),
		Valid:              true,
	}
}

// ToNullStageStatus 将阶段状态转换为 sqlc NullCatalogStageStatus。
func ToNullStageStatus(value *po.StageStatus) catalogsql.NullCatalogStageStatus {
	if value == nil {
		return catalogsql.NullCatalogStageStatus{}
	}
	return catalogsql.NullCatalogStageStatus{
		CatalogStageStatus: catalogsql.CatalogStageStatus(*value),
		Valid:              true,
	}
}

// ToPgTimestamptz 将 time 指针转换为 pgtype.Timestamptz。
func ToPgTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{
		Time:  value.UTC(),
		Valid: true,
	}
}

// BuildUpsertVideoUserStateParams 构造写入用户互动状态的 sqlc 参数。
func BuildUpsertVideoUserStateParams(
	userID uuid.UUID,
	videoID uuid.UUID,
	hasLiked bool,
	hasBookmarked bool,
	likedOccurredAt *time.Time,
	bookmarkedOccurredAt *time.Time,
) catalogsql.UpsertVideoUserStateParams {
	return catalogsql.UpsertVideoUserStateParams{
		UserID:               userID,
		VideoID:              videoID,
		HasLiked:             hasLiked,
		HasBookmarked:        hasBookmarked,
		LikedOccurredAt:      ToPgTimestamptz(likedOccurredAt),
		BookmarkedOccurredAt: ToPgTimestamptz(bookmarkedOccurredAt),
	}
}
