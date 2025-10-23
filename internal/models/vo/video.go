// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
)

// VideoDetail 封装视频完整元数据。
// 用于 GetVideoMetadata / GetVideoDetail RPC 响应。
type VideoDetail struct {
	VideoID        uuid.UUID `json:"video_id"`
	Title          string    `json:"title"`
	Description    *string   `json:"description"`
	Status         string    `json:"status"`
	MediaStatus    string    `json:"media_status"`
	AnalysisStatus string    `json:"analysis_status"`
	ThumbnailURL   *string   `json:"thumbnail_url"`
	DurationMicros *int64    `json:"duration_micros"`
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NewVideoDetail 从领域实体构造 VO，隔离底层存储模型。
func NewVideoDetail(video *po.Video) *VideoDetail {
	if video == nil {
		return nil
	}
	return &VideoDetail{
		VideoID:        video.VideoID,
		Title:          video.Title,
		Description:    video.Description,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
		ThumbnailURL:   video.ThumbnailURL,
		DurationMicros: video.DurationMicros,
		Tags:           append([]string(nil), video.Tags...),
		CreatedAt:      video.CreatedAt,
		UpdatedAt:      video.UpdatedAt,
	}
}
