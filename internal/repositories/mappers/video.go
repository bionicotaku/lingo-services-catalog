// Package mappers 提供仓储层的模型转换工具，将存储层结果映射为领域实体。
package mappers

import (
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// BuildCreateVideoParams 将仓储层输入转换为 sqlc CreateVideoParams，统一处理可空字段。
func BuildCreateVideoParams(uploadUserID uuid.UUID, title, rawFileReference string, description *string) catalogsql.CreateVideoParams {
	return catalogsql.CreateVideoParams{
		UploadUserID:     uploadUserID,
		Title:            title,
		RawFileReference: rawFileReference,
		Description:      textFromPtr(description),
	}
}

// BuildInsertOutboxEventParams 构造 sqlc InsertOutboxEventParams，确保时间与可空字段一致。
func BuildInsertOutboxEventParams(eventID uuid.UUID, aggregateType string, aggregateID uuid.UUID, eventType string, payload, headers []byte, availableAt time.Time) catalogsql.InsertOutboxEventParams {
	return catalogsql.InsertOutboxEventParams{
		EventID:       eventID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       payload,
		Headers:       headers,
		AvailableAt:   timestamptzFromTime(availableAt),
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
		MediaStatus:       po.StageStatus(v.MediaStatus),
		AnalysisStatus:    po.StageStatus(v.AnalysisStatus),
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
		RawSubtitleURL:    textPtr(v.RawSubtitleUrl),
		ErrorMessage:      textPtr(v.ErrorMessage),
	}
}

// VideoReadyViewFromCatalog 将 sqlc 生成的 CatalogVideosReadyView 转换为 po.VideoReadyView。
func VideoReadyViewFromCatalog(v catalogsql.CatalogVideosReadyView) *po.VideoReadyView {
	return &po.VideoReadyView{
		VideoID:        v.VideoID,
		Title:          v.Title,
		Status:         po.VideoStatus(v.Status),
		MediaStatus:    po.StageStatus(v.MediaStatus),
		AnalysisStatus: po.StageStatus(v.AnalysisStatus),
		CreatedAt:      mustTimestamp(v.CreatedAt),
		UpdatedAt:      mustTimestamp(v.UpdatedAt),
	}
}

func mustTimestamp(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
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

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{
		Time:  t.UTC(),
		Valid: true,
	}
}
