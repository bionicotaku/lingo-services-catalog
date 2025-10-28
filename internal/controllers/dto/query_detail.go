package dto

import (
	"fmt"

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

// NewGetVideoMetadataResponse 将 VO 层元数据转换为 gRPC 响应。
func NewGetVideoMetadataResponse(meta *vo.VideoMetadata) *videov1.GetVideoMetadataResponse {
	return &videov1.GetVideoMetadataResponse{
		Metadata: NewVideoMetadata(meta),
	}
}

// NewGetVideoDetailResponse 将详情 + 元数据组合为 gRPC 响应。
func NewGetVideoDetailResponse(detail *vo.VideoDetail, meta *vo.VideoMetadata) *videov1.GetVideoDetailResponse {
	return &videov1.GetVideoDetailResponse{
		Detail:   NewVideoDetail(detail),
		Metadata: NewVideoMetadata(meta),
	}
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
	}
}

// NewVideoMetadata 将元数据 VO 转换为 Proto。
func NewVideoMetadata(meta *vo.VideoMetadata) *videov1.VideoMetadata {
	if meta == nil {
		return &videov1.VideoMetadata{}
	}
	return &videov1.VideoMetadata{
		Status:            meta.Status,
		MediaStatus:       meta.MediaStatus,
		AnalysisStatus:    meta.AnalysisStatus,
		DurationMicros:    meta.DurationMicros,
		EncodedResolution: meta.EncodedResolution,
		EncodedBitrate:    meta.EncodedBitrate,
		ThumbnailUrl:      meta.ThumbnailURL,
		HlsMasterPlaylist: meta.HLSMasterPlaylist,
		Difficulty:        meta.Difficulty,
		Summary:           meta.Summary,
		Tags:              append([]string(nil), meta.Tags...),
		RawSubtitleUrl:    meta.RawSubtitleURL,
		UpdatedAt:         FormatTime(meta.UpdatedAt),
		Version:           meta.Version,
	}
}
