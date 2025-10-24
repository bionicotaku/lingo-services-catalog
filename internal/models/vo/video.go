// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
)

// VideoDetail 封装视频精简视图，仅包含前端展示需要的核心字段。
// 用于 GetVideoDetail RPC 响应。
type VideoDetail struct {
	VideoID     uuid.UUID `json:"video_id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	Status      string    `json:"status"`

	// 播放相关
	ThumbnailURL      *string `json:"thumbnail_url"`
	HLSMasterPlaylist *string `json:"hls_master_playlist"`
	DurationMicros    *int64  `json:"duration_micros"`

	// AI 分析结果
	Difficulty *string  `json:"difficulty"`
	Summary    *string  `json:"summary"`
	Tags       []string `json:"tags"`

	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewVideoDetail 从领域实体构造精简 VO，只包含前端需要的核心字段。
func NewVideoDetail(video *po.Video) *VideoDetail {
	if video == nil {
		return nil
	}
	return &VideoDetail{
		VideoID:     video.VideoID,
		Title:       video.Title,
		Description: video.Description,
		Status:      string(video.Status),

		// 播放相关
		ThumbnailURL:      video.ThumbnailURL,
		HLSMasterPlaylist: video.HLSMasterPlaylist,
		DurationMicros:    video.DurationMicros,

		// AI 分析结果
		Difficulty: video.Difficulty,
		Summary:    video.Summary,
		Tags:       append([]string(nil), video.Tags...), // 防御性拷贝

		// 时间戳
		CreatedAt: video.CreatedAt,
		UpdatedAt: video.UpdatedAt,
	}
}
