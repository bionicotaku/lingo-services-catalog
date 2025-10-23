// Package views 负责将内部 VO 对象转换为 gRPC 响应。
// 该层作为传输层的序列化适配器，隔离业务逻辑与协议细节。
package views

import (
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewGetVideoDetailResponse 将 VideoDetail 视图对象转换为 gRPC 响应。
func NewGetVideoDetailResponse(detail *vo.VideoDetail) *videov1.GetVideoDetailResponse {
	return &videov1.GetVideoDetailResponse{Detail: NewVideoDetail(detail)}
}

// NewVideoDetail 将 VideoDetail 视图对象转换为 gRPC DTO。
func NewVideoDetail(detail *vo.VideoDetail) *videov1.VideoDetail {
	if detail == nil {
		return &videov1.VideoDetail{}
	}

	resp := &videov1.VideoDetail{
		VideoId:        detail.VideoID.String(),
		Title:          detail.Title,
		Status:         detail.Status,
		MediaStatus:    detail.MediaStatus,
		AnalysisStatus: detail.AnalysisStatus,
		Tags:           append([]string(nil), detail.Tags...),
		CreatedAt:      timestamppb.New(detail.CreatedAt),
		UpdatedAt:      timestamppb.New(detail.UpdatedAt),
	}

	if detail.Description != nil {
		resp.Description = wrapperspb.String(*detail.Description)
	}
	if detail.ThumbnailURL != nil {
		resp.ThumbnailUrl = wrapperspb.String(*detail.ThumbnailURL)
	}
	if detail.DurationMicros != nil {
		resp.DurationMicros = wrapperspb.Int64(*detail.DurationMicros)
	}

	return resp
}
