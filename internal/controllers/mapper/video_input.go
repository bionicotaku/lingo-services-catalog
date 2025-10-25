// Package mapper 提供 gRPC 请求到 service 输入的转换能力。
package mapper

import (
	"fmt"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/services"
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

// ParseVideoID 解析通用的 video_id 字段。
func ParseVideoID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid video_id: %w", err)
	}
	return id, nil
}
