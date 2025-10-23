// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	"github.com/google/uuid"
)

// VideoRevision 封装视频状态变更后返回的关键信息。
// 用于 Lifecycle RPC 的响应（RegisterUpload, UpdateMediaInfo 等）。
type VideoRevision struct {
	VideoID   uuid.UUID `json:"video_id"`   // 视频 ID
	Status    string    `json:"status"`     // 当前总体状态
	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 最近更新时间
}

// VideoDetail 封装视频完整元数据。
// 用于 GetVideoMetadata / GetVideoDetail RPC 响应。
type VideoDetail struct {
	VideoID        uuid.UUID  `json:"video_id"`        // 视频 ID
	Title          string     `json:"title"`           // 标题
	Description    *string    `json:"description"`     // 描述（可选）
	Status         string     `json:"status"`          // 总体状态
	MediaStatus    string     `json:"media_status"`    // 媒体阶段状态
	AnalysisStatus string     `json:"analysis_status"` // AI 分析阶段状态
	ThumbnailURL   *string    `json:"thumbnail_url"`   // 缩略图 URL
	DurationMicros *int64     `json:"duration_micros"` // 视频时长（微秒）
	CreatedAt      time.Time  `json:"created_at"`      // 创建时间
	UpdatedAt      time.Time  `json:"updated_at"`      // 最近更新时间
}
