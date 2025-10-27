package dto

import (
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"

	"github.com/google/uuid"
)

// ParseVideoID 解析 video_id 字段。
func ParseVideoID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid video_id: %w", err)
	}
	return id, nil
}

// NewGetVideoDetailResponse 将 VideoDetail 视图对象转换为 gRPC 响应。
func NewGetVideoDetailResponse(detail *vo.VideoDetail) *videov1.GetVideoDetailResponse {
	return &videov1.GetVideoDetailResponse{Detail: NewVideoDetail(detail)}
}

// NewVideoDetail 将 VideoDetail 视图对象转换为 gRPC DTO。
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
		CreatedAt:      FormatTime(detail.CreatedAt),
		UpdatedAt:      FormatTime(detail.UpdatedAt),
		HasLiked:       detail.HasLiked,
		HasBookmarked:  detail.HasBookmarked,
		HasWatched:     detail.HasWatched,
	}
}

// NewVideoListItems 将 VO 列表转换为 proto。
func NewVideoListItems(items []vo.VideoListItem) []*videov1.VideoListItem {
	result := make([]*videov1.VideoListItem, 0, len(items))
	for _, it := range items {
		result = append(result, &videov1.VideoListItem{
			VideoId:        it.VideoID.String(),
			Title:          it.Title,
			Status:         it.Status,
			MediaStatus:    it.MediaStatus,
			AnalysisStatus: it.AnalysisStatus,
			CreatedAt:      FormatTime(it.CreatedAt),
			UpdatedAt:      FormatTime(it.UpdatedAt),
		})
	}
	return result
}

// NewMyUploadListItems 将用户上传列表转换为 proto。
func NewMyUploadListItems(items []vo.MyUploadListItem) []*videov1.MyUploadListItem {
	result := make([]*videov1.MyUploadListItem, 0, len(items))
	for _, it := range items {
		result = append(result, &videov1.MyUploadListItem{
			VideoId:        it.VideoID.String(),
			Title:          it.Title,
			Status:         it.Status,
			MediaStatus:    it.MediaStatus,
			AnalysisStatus: it.AnalysisStatus,
			Version:        it.Version,
			CreatedAt:      FormatTime(it.CreatedAt),
			UpdatedAt:      FormatTime(it.UpdatedAt),
		})
	}
	return result
}

// FormatTime 将时间转换为 RFC3339Nano 字符串，零值返回空串。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
