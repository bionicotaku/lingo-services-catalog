// Package vo 定义视图对象（View Objects），用于向上层传递业务数据。
// VO 对象由 Service 层返回，经 Views 层转换为 API 响应，隔离内部数据结构。
package vo

import (
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
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
	EventID        uuid.UUID `json:"event_id"`
	Version        int64     `json:"version"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// NewVideoCreated 从领域实体构造创建响应 VO。
func NewVideoCreated(video *po.Video, eventID uuid.UUID, version int64, occurredAt time.Time) *VideoCreated {
	if video == nil {
		return nil
	}
	return &VideoCreated{
		VideoID:        video.VideoID,
		CreatedAt:      video.CreatedAt,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
		EventID:        eventID,
		Version:        version,
		OccurredAt:     occurredAt,
	}
}

// VideoDetail 封装视频只读视图，仅包含 ready/published 状态视频的核心信息。
// 用于 GetVideoDetail RPC 响应。
// 数据来源：catalog.videos 主表
type VideoDetail struct {
	VideoID        uuid.UUID `json:"video_id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	MediaStatus    string    `json:"media_status"`
	AnalysisStatus string    `json:"analysis_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HasLiked       bool      `json:"has_liked"`
	HasBookmarked  bool      `json:"has_bookmarked"`
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

// VideoMetadata 表示独立的媒体/AI 元数据视图。
type VideoMetadata struct {
	Status            string    `json:"status"`
	MediaStatus       string    `json:"media_status"`
	AnalysisStatus    string    `json:"analysis_status"`
	DurationMicros    int64     `json:"duration_micros"`
	EncodedResolution string    `json:"encoded_resolution"`
	EncodedBitrate    int32     `json:"encoded_bitrate"`
	ThumbnailURL      string    `json:"thumbnail_url"`
	HLSMasterPlaylist string    `json:"hls_master_playlist"`
	Difficulty        string    `json:"difficulty"`
	Summary           string    `json:"summary"`
	Tags              []string  `json:"tags"`
	RawSubtitleURL    string    `json:"raw_subtitle_url"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           int64     `json:"version"`
}

// NewVideoMetadataFromPO 将持久层元数据转换为 VO。
func NewVideoMetadataFromPO(meta *po.VideoMetadata) *VideoMetadata {
	if meta == nil {
		return nil
	}
	vm := &VideoMetadata{
		Status:         string(meta.Status),
		MediaStatus:    string(meta.MediaStatus),
		AnalysisStatus: string(meta.AnalysisStatus),
		UpdatedAt:      meta.UpdatedAt,
		Version:        meta.Version,
	}
	if meta.DurationMicros != nil {
		vm.DurationMicros = *meta.DurationMicros
	}
	if meta.EncodedResolution != nil {
		vm.EncodedResolution = *meta.EncodedResolution
	}
	if meta.EncodedBitrate != nil {
		vm.EncodedBitrate = *meta.EncodedBitrate
	}
	if meta.ThumbnailURL != nil {
		vm.ThumbnailURL = *meta.ThumbnailURL
	}
	if meta.HLSMasterPlaylist != nil {
		vm.HLSMasterPlaylist = *meta.HLSMasterPlaylist
	}
	if meta.Difficulty != nil {
		vm.Difficulty = *meta.Difficulty
	}
	if meta.Summary != nil {
		vm.Summary = *meta.Summary
	}
	if meta.Tags != nil {
		vm.Tags = append([]string(nil), meta.Tags...)
	}
	if meta.RawSubtitleURL != nil {
		vm.RawSubtitleURL = *meta.RawSubtitleURL
	}
	return vm
}

// VideoListItem 表示公开视频列表中的条目。
type VideoListItem struct {
	VideoID        uuid.UUID
	Title          string
	Status         string
	MediaStatus    string
	AnalysisStatus string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MyUploadListItem 表示用户上传列表中的项。
type MyUploadListItem struct {
	VideoID        uuid.UUID
	Title          string
	Status         string
	MediaStatus    string
	AnalysisStatus string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// VideoUpdated 封装视频更新后的响应信息。
type VideoUpdated struct {
	VideoID        uuid.UUID `json:"video_id"`
	UpdatedAt      time.Time `json:"updated_at"`
	Status         string    `json:"status"`
	MediaStatus    string    `json:"media_status"`
	AnalysisStatus string    `json:"analysis_status"`
	EventID        uuid.UUID `json:"event_id"`
	Version        int64     `json:"version"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// NewVideoUpdated 根据领域实体构造更新响应 VO。
func NewVideoUpdated(video *po.Video, eventID uuid.UUID, version int64, occurredAt time.Time) *VideoUpdated {
	if video == nil {
		return nil
	}
	return &VideoUpdated{
		VideoID:        video.VideoID,
		UpdatedAt:      video.UpdatedAt,
		Status:         string(video.Status),
		MediaStatus:    string(video.MediaStatus),
		AnalysisStatus: string(video.AnalysisStatus),
		EventID:        eventID,
		Version:        version,
		OccurredAt:     occurredAt,
	}
}

// VideoDeleted 封装视频删除后的响应信息。
type VideoDeleted struct {
	VideoID    uuid.UUID `json:"video_id"`
	DeletedAt  time.Time `json:"deleted_at"`
	EventID    uuid.UUID `json:"event_id"`
	Version    int64     `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
}

// NewVideoDeleted 构造删除响应 VO。
func NewVideoDeleted(videoID uuid.UUID, deletedAt time.Time, eventID uuid.UUID, version int64, occurredAt time.Time) *VideoDeleted {
	return &VideoDeleted{
		VideoID:    videoID,
		DeletedAt:  deletedAt,
		EventID:    eventID,
		Version:    version,
		OccurredAt: occurredAt,
	}
}
