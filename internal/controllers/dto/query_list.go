package dto

import (
	"fmt"
	"strings"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
)

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

// ParseStatusFilters 校验并转换视频状态过滤条件。
func ParseStatusFilters(raw []string) ([]po.VideoStatus, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[po.VideoStatus]struct{}, len(raw))
	result := make([]po.VideoStatus, 0, len(raw))
	for _, item := range raw {
		status := po.VideoStatus(strings.ToLower(strings.TrimSpace(item)))
		switch status {
		case po.VideoStatusPendingUpload,
			po.VideoStatusProcessing,
			po.VideoStatusReady,
			po.VideoStatusPublished,
			po.VideoStatusFailed,
			po.VideoStatusRejected,
			po.VideoStatusArchived:
			if _, ok := seen[status]; !ok {
				seen[status] = struct{}{}
				result = append(result, status)
			}
		default:
			return nil, fmt.Errorf("invalid status_filter value: %s", item)
		}
	}
	return result, nil
}

// ParseStageFilters 校验并转换阶段状态过滤条件。
func ParseStageFilters(raw []string) ([]po.StageStatus, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[po.StageStatus]struct{}, len(raw))
	result := make([]po.StageStatus, 0, len(raw))
	for _, item := range raw {
		stage := po.StageStatus(strings.ToLower(strings.TrimSpace(item)))
		switch stage {
		case po.StagePending, po.StageProcessing, po.StageReady, po.StageFailed:
			if _, ok := seen[stage]; !ok {
				seen[stage] = struct{}{}
				result = append(result, stage)
			}
		default:
			return nil, fmt.Errorf("invalid stage_filter value: %s", item)
		}
	}
	return result, nil
}

// FormatTime 将时间转换为 RFC3339Nano 字符串，零值返回空串。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
