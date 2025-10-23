package vo_test

import (
	"testing"
	"time"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVideoDetail(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()
	uploadUserID := uuid.New()

	tests := []struct {
		name     string
		input    *po.Video
		expected *vo.VideoDetail
	}{
		{
			name: "完整 Video 转换",
			input: &po.Video{
				VideoID:           videoID,
				UploadUserID:      uploadUserID,
				CreatedAt:         now,
				UpdatedAt:         now.Add(time.Hour),
				Title:             "测试视频",
				Description:       stringPtr("这是描述"),
				RawFileReference:  "gs://bucket/video.mp4",
				Status:            po.VideoStatusPublished,
				MediaStatus:       po.StageReady,
				AnalysisStatus:    po.StageReady,
				RawFileSize:       int64Ptr(2048000),
				RawResolution:     stringPtr("1920x1080"),
				RawBitrate:        int32Ptr(5000),
				DurationMicros:    int64Ptr(180000000), // 180 秒
				EncodedResolution: stringPtr("1280x720"),
				EncodedBitrate:    int32Ptr(3000),
				ThumbnailURL:      stringPtr("https://cdn.example.com/thumb.jpg"),
				HLSMasterPlaylist: stringPtr("https://cdn.example.com/master.m3u8"),
				Difficulty:        stringPtr("advanced"),
				Summary:           stringPtr("视频摘要"),
				Tags:              []string{"go", "testing", "tutorial"},
				RawSubtitleURL:    stringPtr("gs://bucket/subtitle.vtt"),
				ErrorMessage:      nil,
			},
			expected: &vo.VideoDetail{
				VideoID:        videoID,
				Title:          "测试视频",
				Description:    stringPtr("这是描述"),
				Status:         "published",
				MediaStatus:    "ready",
				AnalysisStatus: "ready",
				ThumbnailURL:   stringPtr("https://cdn.example.com/thumb.jpg"),
				DurationMicros: int64Ptr(180000000),
				Tags:           []string{"go", "testing", "tutorial"},
				CreatedAt:      now,
				UpdatedAt:      now.Add(time.Hour),
			},
		},
		{
			name: "最小必填字段",
			input: &po.Video{
				VideoID:          videoID,
				UploadUserID:     uploadUserID,
				CreatedAt:        now,
				UpdatedAt:        now,
				Title:            "最小视频",
				RawFileReference: "gs://bucket/minimal.mp4",
				Status:           po.VideoStatusPendingUpload,
				MediaStatus:      po.StagePending,
				AnalysisStatus:   po.StagePending,
				Tags:             []string{},
			},
			expected: &vo.VideoDetail{
				VideoID:        videoID,
				Title:          "最小视频",
				Description:    nil,
				Status:         "pending_upload",
				MediaStatus:    "pending",
				AnalysisStatus: "pending",
				ThumbnailURL:   nil,
				DurationMicros: nil,
				Tags:           nil, // append([]string(nil), []string{}...) 返回 nil
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		{
			name:     "nil 输入",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vo.NewVideoDetail(tt.input)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)

			// 比较基本字段
			assert.Equal(t, tt.expected.VideoID, result.VideoID)
			assert.Equal(t, tt.expected.Title, result.Title)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.MediaStatus, result.MediaStatus)
			assert.Equal(t, tt.expected.AnalysisStatus, result.AnalysisStatus)

			// 比较指针字段
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.ThumbnailURL, result.ThumbnailURL)
			assert.Equal(t, tt.expected.DurationMicros, result.DurationMicros)

			// 比较时间字段
			assert.WithinDuration(t, tt.expected.CreatedAt, result.CreatedAt, time.Millisecond)
			assert.WithinDuration(t, tt.expected.UpdatedAt, result.UpdatedAt, time.Millisecond)

			// 比较 Tags
			assert.Equal(t, tt.expected.Tags, result.Tags)
		})
	}
}

func TestNewVideoDetail_TagsDefensiveCopy(t *testing.T) {
	// 测试 Tags 是否做了防御性拷贝
	videoID := uuid.New()
	uploadUserID := uuid.New()
	now := time.Now().UTC()

	originalTags := []string{"golang", "backend"}
	video := &po.Video{
		VideoID:          videoID,
		UploadUserID:     uploadUserID,
		CreatedAt:        now,
		UpdatedAt:        now,
		Title:            "测试",
		RawFileReference: "gs://bucket/test.mp4",
		Status:           po.VideoStatusReady,
		MediaStatus:      po.StageReady,
		AnalysisStatus:   po.StageReady,
		Tags:             originalTags,
	}

	detail := vo.NewVideoDetail(video)

	// 修改返回的 Tags，不应该影响原始数据
	detail.Tags[0] = "modified"

	assert.NotEqual(t, originalTags[0], "modified", "Tags 应该是深拷贝")
	assert.Equal(t, "golang", video.Tags[0], "原始 Tags 不应该被修改")
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
