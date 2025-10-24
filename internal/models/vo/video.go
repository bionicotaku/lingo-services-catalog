// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/google/uuid"
)

// VideoCreated 封装视频创建响应，包含创建后的核心信息。
// 用于 CreateVideo RPC 响应。
type VideoCreated struct {
	VideoID        uuid.UUID `json:"video_id"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
	MediaStatus    string    `json:"media_status"`
	AnalysisStatus string    `json:"analysis_status"`
}

// NewVideoCreated 从领域实体构造创建响应 VO。
func NewVideoCreated(video *po.Video) *VideoCreated {
	if video == nil {
		return nil
	}
	return &VideoCreated{
		VideoID:        video.VideoID,
		CreatedAt:      video.CreatedAt,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
	}
}

// VideoDetail 封装视频只读视图，仅包含 ready/published 状态视频的核心信息。
// 用于 GetVideoDetail RPC 响应。
// 数据来源：catalog.videos_ready_view 视图
type VideoDetail struct {
	VideoID        uuid.UUID `json:"video_id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	MediaStatus    string    `json:"media_status"`
	AnalysisStatus string    `json:"analysis_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NewVideoDetail 从只读视图实体构造 VO。
func NewVideoDetail(video *po.VideoReadyView) *VideoDetail {
	if video == nil {
		return nil
	}
	return &VideoDetail{
		VideoID:        video.VideoID,
		Title:          video.Title,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
		CreatedAt:      video.CreatedAt,
		UpdatedAt:      video.UpdatedAt,
	}
}
