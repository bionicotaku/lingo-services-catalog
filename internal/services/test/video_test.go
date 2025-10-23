package services_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	kratosErrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockVideoRepo 是 VideoRepo 接口的 mock 实现
type MockVideoRepo struct {
	mock.Mock
}

func (m *MockVideoRepo) FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error) {
	args := m.Called(ctx, videoID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*po.Video), args.Error(1)
}

func TestVideoUsecase_GetVideoDetail(t *testing.T) {
	logger := log.NewStdLogger(io.Discard) // 测试时不输出日志
	videoID := uuid.New()
	uploadUserID := uuid.New()
	now := time.Now().UTC()

	tests := []struct {
		name          string
		videoID       uuid.UUID
		mockSetup     func(*MockVideoRepo)
		wantErr       bool
		checkError    func(t *testing.T, err error)
		checkResult   func(t *testing.T, result *services.VideoUsecase, videoID uuid.UUID)
	}{
		{
			name:    "成功获取视频详情",
			videoID: videoID,
			mockSetup: func(repo *MockVideoRepo) {
				video := &po.Video{
					VideoID:           videoID,
					UploadUserID:      uploadUserID,
					CreatedAt:         now,
					UpdatedAt:         now,
					Title:             "测试视频",
					Description:       stringPtr("视频描述"),
					RawFileReference:  "gs://bucket/video.mp4",
					Status:            po.VideoStatusPublished,
					MediaStatus:       po.StageReady,
					AnalysisStatus:    po.StageReady,
					ThumbnailURL:      stringPtr("https://cdn.example.com/thumb.jpg"),
					DurationMicros:    int64Ptr(120000000),
					Tags:              []string{"golang", "tutorial"},
				}
				repo.On("FindByID", mock.Anything, videoID).Return(video, nil)
			},
			wantErr: false,
			checkResult: func(t *testing.T, uc *services.VideoUsecase, videoID uuid.UUID) {
				detail, err := uc.GetVideoDetail(context.Background(), videoID)
				require.NoError(t, err)
				require.NotNil(t, detail)

				assert.Equal(t, videoID, detail.VideoID)
				assert.Equal(t, "测试视频", detail.Title)
				assert.Equal(t, stringPtr("视频描述"), detail.Description)
				assert.Equal(t, "published", detail.Status)
				assert.Equal(t, []string{"golang", "tutorial"}, detail.Tags)
			},
		},
		{
			name:    "视频不存在",
			videoID: videoID,
			mockSetup: func(repo *MockVideoRepo) {
				repo.On("FindByID", mock.Anything, videoID).Return(nil, repositories.ErrVideoNotFound)
			},
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.True(t, kratosErrors.IsNotFound(err), "应该返回 404 错误")
				assert.Equal(t, videov1.ErrorReason_VIDEO_NOT_FOUND.String(), kratosErrors.Reason(err))
			},
		},
		{
			name:    "查询超时",
			videoID: videoID,
			mockSetup: func(repo *MockVideoRepo) {
				repo.On("FindByID", mock.Anything, videoID).Return(nil, context.DeadlineExceeded)
			},
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.True(t, kratosErrors.IsGatewayTimeout(err), "应该返回 504 超时错误")
				assert.Equal(t, videov1.ErrorReason_QUERY_TIMEOUT.String(), kratosErrors.Reason(err))
			},
		},
		{
			name:    "数据库内部错误",
			videoID: videoID,
			mockSetup: func(repo *MockVideoRepo) {
				repo.On("FindByID", mock.Anything, videoID).Return(nil, errors.New("database connection lost"))
			},
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.True(t, kratosErrors.IsInternalServer(err), "应该返回 500 内部错误")
				assert.Equal(t, videov1.ErrorReason_QUERY_VIDEO_FAILED.String(), kratosErrors.Reason(err))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 mock repository
			repo := new(MockVideoRepo)
			tt.mockSetup(repo)

			// 创建 usecase
			uc := services.NewVideoUsecase(repo, logger)

			if tt.wantErr {
				_, err := uc.GetVideoDetail(context.Background(), tt.videoID)
				tt.checkError(t, err)
			} else {
				tt.checkResult(t, uc, tt.videoID)
			}

			// 验证 mock 调用
			repo.AssertExpectations(t)
		})
	}
}

func TestVideoUsecase_GetVideoDetail_ContextCancellation(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoID := uuid.New()
	repo := new(MockVideoRepo)

	// 模拟 context 被取消
	repo.On("FindByID", mock.Anything, videoID).Return(nil, context.Canceled)

	uc := services.NewVideoUsecase(repo, logger)

	_, err := uc.GetVideoDetail(context.Background(), videoID)

	require.Error(t, err)
	assert.True(t, kratosErrors.IsInternalServer(err), "取消的 context 应该作为内部错误处理")

	repo.AssertExpectations(t)
}

// 辅助函数
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
