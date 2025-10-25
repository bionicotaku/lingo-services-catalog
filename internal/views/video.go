// Package views 负责将内部 VO 对象转换为 gRPC 响应。
// 该层作为传输层的序列化适配器，隔离业务逻辑与协议细节。
package views

import (
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"
)

// NewCreateVideoResponse 将 VideoCreated 视图对象转换为 gRPC 响应。
func NewCreateVideoResponse(created *vo.VideoCreated) *videov1.CreateVideoResponse {
	if created == nil {
		return &videov1.CreateVideoResponse{}
	}
	return &videov1.CreateVideoResponse{
		VideoId:        created.VideoID.String(),
		CreatedAt:      formatTime(created.CreatedAt),
		Status:         created.Status,
		MediaStatus:    created.MediaStatus,
		AnalysisStatus: created.AnalysisStatus,
		EventId:        created.EventID.String(),
		Version:        created.Version,
		OccurredAt:     formatTime(created.OccurredAt),
	}
}

// NewGetVideoDetailResponse 将 VideoDetail 视图对象转换为 gRPC 响应。
func NewGetVideoDetailResponse(detail *vo.VideoDetail) *videov1.GetVideoDetailResponse {
	return &videov1.GetVideoDetailResponse{Detail: NewVideoDetail(detail)}
}

// NewVideoDetail 将 VideoDetail 视图对象转换为 gRPC DTO。
// 只包含只读视图中的字段（ready/published 状态视频的核心信息）。
func NewVideoDetail(detail *vo.VideoDetail) *videov1.VideoDetail {
	if detail == nil {
		return &videov1.VideoDetail{}
	}

	return &videov1.VideoDetail{
		VideoId:        detail.VideoID.String(),
		Title:          detail.Title,
		Status:         detail.Status,
		MediaStatus:    detail.MediaStatus,
		AnalysisStatus: detail.AnalysisStatus,
		CreatedAt:      formatTime(detail.CreatedAt),
		UpdatedAt:      formatTime(detail.UpdatedAt),
	}
}

// NewUpdateVideoResponse 将更新后的 VO 转换为 gRPC 响应。
func NewUpdateVideoResponse(updated *vo.VideoUpdated) *videov1.UpdateVideoResponse {
	if updated == nil {
		return &videov1.UpdateVideoResponse{}
	}
	return &videov1.UpdateVideoResponse{
		VideoId:        updated.VideoID.String(),
		UpdatedAt:      formatTime(updated.UpdatedAt),
		Status:         updated.Status,
		MediaStatus:    updated.MediaStatus,
		AnalysisStatus: updated.AnalysisStatus,
		EventId:        updated.EventID.String(),
		Version:        updated.Version,
		OccurredAt:     formatTime(updated.OccurredAt),
	}
}

// NewDeleteVideoResponse 将删除结果转换为 gRPC 响应。
func NewDeleteVideoResponse(deleted *vo.VideoDeleted) *videov1.DeleteVideoResponse {
	if deleted == nil {
		return &videov1.DeleteVideoResponse{}
	}
	return &videov1.DeleteVideoResponse{
		VideoId:    deleted.VideoID.String(),
		DeletedAt:  formatTime(deleted.DeletedAt),
		EventId:    deleted.EventID.String(),
		Version:    deleted.Version,
		OccurredAt: formatTime(deleted.OccurredAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
