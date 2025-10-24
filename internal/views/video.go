// Package views 负责将内部 VO 对象转换为 gRPC 响应。
// 该层作为传输层的序列化适配器，隔离业务逻辑与协议细节。
package views

import (
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// NewCreateVideoResponse 将 VideoCreated 视图对象转换为 gRPC 响应。
func NewCreateVideoResponse(created *vo.VideoCreated) *videov1.CreateVideoResponse {
	if created == nil {
		return &videov1.CreateVideoResponse{}
	}
	return &videov1.CreateVideoResponse{
		VideoId:        created.VideoID.String(),
		CreatedAt:      timestamppb.New(created.CreatedAt),
		Status:         created.Status,
		MediaStatus:    created.MediaStatus,
		AnalysisStatus: created.AnalysisStatus,
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
		CreatedAt:      timestamppb.New(detail.CreatedAt),
		UpdatedAt:      timestamppb.New(detail.UpdatedAt),
	}
}
