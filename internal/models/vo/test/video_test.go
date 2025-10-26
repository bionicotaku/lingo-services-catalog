package vo_test

import (
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/vo"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVideoDetail(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()

	tests := []struct {
		name     string
		input    *po.VideoReadyView
		expected *vo.VideoDetail
	}{
		{
			name: "完整 VideoReadyView 转换 - published 状态",
			input: &po.VideoReadyView{
				VideoID:        videoID,
				Title:          "测试视频",
				Status:         po.VideoStatusPublished,
				MediaStatus:    po.StageReady,
				AnalysisStatus: po.StageReady,
				CreatedAt:      now,
				UpdatedAt:      now.Add(time.Hour),
			},
			expected: &vo.VideoDetail{
				VideoID:        videoID,
				Title:          "测试视频",
				Status:         "published",
				MediaStatus:    "ready",
				AnalysisStatus: "ready",
				CreatedAt:      now,
				UpdatedAt:      now.Add(time.Hour),
			},
		},
		{
			name: "VideoReadyView 转换 - ready 状态",
			input: &po.VideoReadyView{
				VideoID:        videoID,
				Title:          "就绪视频",
				Status:         po.VideoStatusReady,
				MediaStatus:    po.StageReady,
				AnalysisStatus: po.StageReady,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expected: &vo.VideoDetail{
				VideoID:        videoID,
				Title:          "就绪视频",
				Status:         "ready",
				MediaStatus:    "ready",
				AnalysisStatus: "ready",
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

			// 比较所有字段
			assert.Equal(t, tt.expected.VideoID, result.VideoID)
			assert.Equal(t, tt.expected.Title, result.Title)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.MediaStatus, result.MediaStatus)
			assert.Equal(t, tt.expected.AnalysisStatus, result.AnalysisStatus)

			// 比较时间字段
			assert.WithinDuration(t, tt.expected.CreatedAt, result.CreatedAt, time.Millisecond)
			assert.WithinDuration(t, tt.expected.UpdatedAt, result.UpdatedAt, time.Millisecond)
		})
	}
}

// TestNewVideoDetail_NilSafety 测试 nil 输入的安全处理
func TestNewVideoDetail_NilSafety(t *testing.T) {
	result := vo.NewVideoDetail(nil)
	assert.Nil(t, result, "nil 输入应该返回 nil")
}

func TestNewVideoCreated(t *testing.T) {
	now := time.Now().UTC()
	eventID := uuid.New()
	video := &po.Video{
		VideoID:        uuid.New(),
		CreatedAt:      now,
		Status:         po.VideoStatusReady,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
	}

	created := vo.NewVideoCreated(video, eventID, 42, now)
	require.NotNil(t, created)
	assert.Equal(t, video.VideoID, created.VideoID)
	assert.Equal(t, eventID, created.EventID)
	assert.Equal(t, int64(42), created.Version)
	assert.Equal(t, now, created.OccurredAt)
}

func TestNewVideoUpdated(t *testing.T) {
	now := time.Now().UTC()
	eventID := uuid.New()
	video := &po.Video{
		VideoID:        uuid.New(),
		UpdatedAt:      now,
		Status:         po.VideoStatusPublished,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
	}

	updated := vo.NewVideoUpdated(video, eventID, 100, now)
	require.NotNil(t, updated)
	assert.Equal(t, video.VideoID, updated.VideoID)
	assert.Equal(t, string(video.Status), updated.Status)
	assert.Equal(t, eventID, updated.EventID)
	assert.Equal(t, int64(100), updated.Version)
	assert.Equal(t, now, updated.OccurredAt)
}

func TestNewVideoDeleted(t *testing.T) {
	now := time.Now().UTC()
	eventID := uuid.New()
	videoID := uuid.New()

	deleted := vo.NewVideoDeleted(videoID, now, eventID, 99, now)
	require.NotNil(t, deleted)
	assert.Equal(t, videoID, deleted.VideoID)
	assert.Equal(t, eventID, deleted.EventID)
	assert.Equal(t, int64(99), deleted.Version)
	assert.Equal(t, now, deleted.OccurredAt)
}
