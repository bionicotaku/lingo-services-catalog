// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// VideoDetail 封装视频完整元数据。
// 用于 GetVideoMetadata / GetVideoDetail RPC 响应。
type VideoDetail struct {
	VideoID        uuid.UUID `json:"video_id"`        // 视频 ID
	Title          string    `json:"title"`           // 标题
	Description    *string   `json:"description"`     // 描述（可选）
	Status         string    `json:"status"`          // 总体状态
	MediaStatus    string    `json:"media_status"`    // 媒体阶段状态
	AnalysisStatus string    `json:"analysis_status"` // AI 分析阶段状态
	ThumbnailURL   *string   `json:"thumbnail_url"`   // 缩略图 URL
	DurationMicros *int64    `json:"duration_micros"` // 视频时长（微秒）
	CreatedAt      time.Time `json:"created_at"`      // 创建时间
	UpdatedAt      time.Time `json:"updated_at"`      // 最近更新时间
}

// NewVideoDetail 从 sqlc 生成的 CatalogVideo 构造 VideoDetail。
// 封装 pgtype 类型转换逻辑，Service 层无需关心转换细节。
func NewVideoDetail(cv *catalogsql.CatalogVideo) *VideoDetail {
	return &VideoDetail{
		VideoID:        cv.VideoID,
		Title:          cv.Title,
		Description:    toStringPtr(cv.Description),
		Status:         string(cv.Status),
		MediaStatus:    string(cv.MediaStatus),
		AnalysisStatus: string(cv.AnalysisStatus),
		ThumbnailURL:   toStringPtr(cv.ThumbnailUrl),
		DurationMicros: toInt64Ptr(cv.DurationMicros),
		CreatedAt:      cv.CreatedAt.Time,
		UpdatedAt:      cv.UpdatedAt.Time,
	}
}

// toStringPtr 将 pgtype.Text 转换为 *string（NULL 映射为 nil）。
func toStringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// toInt64Ptr 将 pgtype.Int8 转换为 *int64（NULL 映射为 nil）。
func toInt64Ptr(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}
