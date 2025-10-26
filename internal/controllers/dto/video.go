// Package dto 提供控制器层的请求解析与响应构造工具。
// 单独的 dto 层可以隔离协议对象与业务用例之间的转换逻辑。
package dto

import (
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/google/uuid"
)

// ToCreateVideoInput 将 gRPC CreateVideoRequest 映射为服务层输入。
func ToCreateVideoInput(req *videov1.CreateVideoRequest) (services.CreateVideoInput, error) {
	uploaderID, err := uuid.Parse(req.GetUploadUserId())
	if err != nil {
		return services.CreateVideoInput{}, fmt.Errorf("invalid upload_user_id: %w", err)
	}

	input := services.CreateVideoInput{
		UploadUserID:     uploaderID,
		Title:            req.GetTitle(),
		RawFileReference: req.GetRawFileReference(),
	}
	if req.Description != nil {
		desc := req.GetDescription()
		input.Description = &desc
	}
	return input, nil
}

// ToUpdateVideoInput 将 UpdateVideoRequest 映射为服务层输入。
func ToUpdateVideoInput(req *videov1.UpdateVideoRequest) (services.UpdateVideoInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.UpdateVideoInput{}, fmt.Errorf("invalid video_id: %w", err)
	}

	input := services.UpdateVideoInput{
		VideoID: videoID,
	}
	if req.Title != nil {
		value := req.GetTitle()
		input.Title = &value
	}
	if req.Description != nil {
		value := req.GetDescription()
		input.Description = &value
	}
	if req.Status != nil {
		value := req.GetStatus()
		input.Status = &value
	}
	if req.MediaStatus != nil {
		value := req.GetMediaStatus()
		input.MediaStatus = &value
	}
	if req.AnalysisStatus != nil {
		value := req.GetAnalysisStatus()
		input.AnalysisStatus = &value
	}
	if req.DurationMicros != nil {
		value := req.GetDurationMicros()
		input.DurationMicros = &value
	}
	if req.ThumbnailUrl != nil {
		value := req.GetThumbnailUrl()
		input.ThumbnailURL = &value
	}
	if req.HlsMasterPlaylist != nil {
		value := req.GetHlsMasterPlaylist()
		input.HLSMasterPlaylist = &value
	}
	if req.Difficulty != nil {
		value := req.GetDifficulty()
		input.Difficulty = &value
	}
	if req.Summary != nil {
		value := req.GetSummary()
		input.Summary = &value
	}
	if req.RawSubtitleUrl != nil {
		value := req.GetRawSubtitleUrl()
		input.RawSubtitleURL = &value
	}
	if req.ErrorMessage != nil {
		value := req.GetErrorMessage()
		input.ErrorMessage = &value
	}
	return input, nil
}

// ToDeleteVideoInput 将 DeleteVideoRequest 映射为服务层输入。
func ToDeleteVideoInput(req *videov1.DeleteVideoRequest) (services.DeleteVideoInput, error) {
	videoID, err := uuid.Parse(req.GetVideoId())
	if err != nil {
		return services.DeleteVideoInput{}, fmt.Errorf("invalid video_id: %w", err)
	}

	input := services.DeleteVideoInput{
		VideoID: videoID,
	}
	if req.Reason != nil {
		value := req.GetReason()
		input.Reason = &value
	}
	return input, nil
}

// ParseVideoID 解析 video_id 字段。
func ParseVideoID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid video_id: %w", err)
	}
	return id, nil
}

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
