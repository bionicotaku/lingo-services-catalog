package mappers_test

import (
	"testing"
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoFromCatalog(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()
	uploadUserID := uuid.New()

	tests := []struct {
		name     string
		input    catalogsql.CatalogVideo
		expected *po.Video
	}{
		{
			name: "完整字段映射",
			input: catalogsql.CatalogVideo{
				VideoID:      videoID,
				UploadUserID: uploadUserID,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
				UpdatedAt: pgtype.Timestamptz{
					Time:  now.Add(time.Hour),
					Valid: true,
				},
				Title: "测试视频",
				Description: pgtype.Text{
					String: "这是一个测试视频",
					Valid:  true,
				},
				RawFileReference: "gs://bucket/raw/video.mp4",
				Status:           po.VideoStatusProcessing,
				MediaStatus:      po.StageProcessing,
				AnalysisStatus:   po.StagePending,
				RawFileSize: pgtype.Int8{
					Int64: 1024000,
					Valid: true,
				},
				RawResolution: pgtype.Text{
					String: "1920x1080",
					Valid:  true,
				},
				RawBitrate: pgtype.Int4{
					Int32: 5000,
					Valid: true,
				},
				DurationMicros: pgtype.Int8{
					Int64: 120000000, // 120 秒
					Valid: true,
				},
				EncodedResolution: pgtype.Text{
					String: "1280x720",
					Valid:  true,
				},
				EncodedBitrate: pgtype.Int4{
					Int32: 3000,
					Valid: true,
				},
				ThumbnailUrl: pgtype.Text{
					String: "https://cdn.example.com/thumb.jpg",
					Valid:  true,
				},
				HlsMasterPlaylist: pgtype.Text{
					String: "https://cdn.example.com/master.m3u8",
					Valid:  true,
				},
				Difficulty: pgtype.Text{
					String: "intermediate",
					Valid:  true,
				},
				Summary: pgtype.Text{
					String: "AI 生成的摘要",
					Valid:  true,
				},
				Tags: []string{"golang", "backend", "tutorial"},
				RawSubtitleUrl: pgtype.Text{
					String: "gs://bucket/subtitles/video.vtt",
					Valid:  true,
				},
				ErrorMessage: pgtype.Text{
					Valid: false,
				},
			},
			expected: &po.Video{
				VideoID:           videoID,
				UploadUserID:      uploadUserID,
				CreatedAt:         now,
				UpdatedAt:         now.Add(time.Hour),
				Title:             "测试视频",
				Description:       stringPtr("这是一个测试视频"),
				RawFileReference:  "gs://bucket/raw/video.mp4",
				Status:            po.VideoStatusProcessing,
				MediaStatus:       po.StageProcessing,
				AnalysisStatus:    po.StagePending,
				RawFileSize:       int64Ptr(1024000),
				RawResolution:     stringPtr("1920x1080"),
				RawBitrate:        int32Ptr(5000),
				DurationMicros:    int64Ptr(120000000),
				EncodedResolution: stringPtr("1280x720"),
				EncodedBitrate:    int32Ptr(3000),
				ThumbnailURL:      stringPtr("https://cdn.example.com/thumb.jpg"),
				HLSMasterPlaylist: stringPtr("https://cdn.example.com/master.m3u8"),
				Difficulty:        stringPtr("intermediate"),
				Summary:           stringPtr("AI 生成的摘要"),
				Tags:              []string{"golang", "backend", "tutorial"},
				RawSubtitleURL:    stringPtr("gs://bucket/subtitles/video.vtt"),
				ErrorMessage:      nil,
			},
		},
		{
			name: "最小必填字段",
			input: catalogsql.CatalogVideo{
				VideoID:      videoID,
				UploadUserID: uploadUserID,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
				UpdatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
				Title:            "最小测试",
				Description:      pgtype.Text{Valid: false},
				RawFileReference: "gs://bucket/raw/minimal.mp4",
				Status:           po.VideoStatusPendingUpload,
				MediaStatus:      po.StagePending,
				AnalysisStatus:   po.StagePending,
				Tags:             []string{},
			},
			expected: &po.Video{
				VideoID:           videoID,
				UploadUserID:      uploadUserID,
				CreatedAt:         now,
				UpdatedAt:         now,
				Title:             "最小测试",
				Description:       nil,
				RawFileReference:  "gs://bucket/raw/minimal.mp4",
				Status:            po.VideoStatusPendingUpload,
				MediaStatus:       po.StagePending,
				AnalysisStatus:    po.StagePending,
				RawFileSize:       nil,
				RawResolution:     nil,
				RawBitrate:        nil,
				DurationMicros:    nil,
				EncodedResolution: nil,
				EncodedBitrate:    nil,
				ThumbnailURL:      nil,
				HLSMasterPlaylist: nil,
				Difficulty:        nil,
				Summary:           nil,
				Tags:              nil, // append([]string(nil), []string{}...) 返回 nil
				RawSubtitleURL:    nil,
				ErrorMessage:      nil,
			},
		},
		{
			name: "包含错误信息",
			input: catalogsql.CatalogVideo{
				VideoID:      videoID,
				UploadUserID: uploadUserID,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
				UpdatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
				Title:            "失败的视频",
				RawFileReference: "gs://bucket/raw/failed.mp4",
				Status:           po.VideoStatusFailed,
				MediaStatus:      po.StageFailed,
				AnalysisStatus:   po.StagePending,
				ErrorMessage: pgtype.Text{
					String: "转码失败: 不支持的编码格式",
					Valid:  true,
				},
				Tags: []string{},
			},
			expected: &po.Video{
				VideoID:          videoID,
				UploadUserID:     uploadUserID,
				CreatedAt:        now,
				UpdatedAt:        now,
				Title:            "失败的视频",
				RawFileReference: "gs://bucket/raw/failed.mp4",
				Status:           po.VideoStatusFailed,
				MediaStatus:      po.StageFailed,
				AnalysisStatus:   po.StagePending,
				ErrorMessage:     stringPtr("转码失败: 不支持的编码格式"),
				Tags:             nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mappers.VideoFromCatalog(tt.input)

			require.NotNil(t, result)

			// 比较基本字段
			assert.Equal(t, tt.expected.VideoID, result.VideoID)
			assert.Equal(t, tt.expected.UploadUserID, result.UploadUserID)
			assert.Equal(t, tt.expected.Title, result.Title)
			assert.Equal(t, tt.expected.RawFileReference, result.RawFileReference)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.MediaStatus, result.MediaStatus)
			assert.Equal(t, tt.expected.AnalysisStatus, result.AnalysisStatus)

			// 比较时间字段（允许微小误差）
			assert.WithinDuration(t, tt.expected.CreatedAt, result.CreatedAt, time.Millisecond)
			assert.WithinDuration(t, tt.expected.UpdatedAt, result.UpdatedAt, time.Millisecond)

			// 比较指针字段
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.RawFileSize, result.RawFileSize)
			assert.Equal(t, tt.expected.RawResolution, result.RawResolution)
			assert.Equal(t, tt.expected.RawBitrate, result.RawBitrate)
			assert.Equal(t, tt.expected.DurationMicros, result.DurationMicros)
			assert.Equal(t, tt.expected.EncodedResolution, result.EncodedResolution)
			assert.Equal(t, tt.expected.EncodedBitrate, result.EncodedBitrate)
			assert.Equal(t, tt.expected.ThumbnailURL, result.ThumbnailURL)
			assert.Equal(t, tt.expected.HLSMasterPlaylist, result.HLSMasterPlaylist)
			assert.Equal(t, tt.expected.Difficulty, result.Difficulty)
			assert.Equal(t, tt.expected.Summary, result.Summary)
			assert.Equal(t, tt.expected.RawSubtitleURL, result.RawSubtitleURL)
			assert.Equal(t, tt.expected.ErrorMessage, result.ErrorMessage)

			// 比较 Tags 切片
			assert.Equal(t, tt.expected.Tags, result.Tags)

			// 验证 Tags 防御性拷贝
			if len(result.Tags) > 0 {
				originalTags := tt.input.Tags
				result.Tags[0] = "modified"
				assert.NotEqual(t, originalTags[0], "modified", "Tags 应该是深拷贝")
			}
		})
	}
}

// 辅助函数
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}
