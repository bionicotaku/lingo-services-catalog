package dto_test

import (
	"testing"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers/dto"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToCreateVideoInput(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		uploaderID := uuid.New()
		description := "Test description"

		req := &videov1.CreateVideoRequest{
			UploadUserId:     uploaderID.String(),
			Title:            "Test Video",
			RawFileReference: "s3://bucket/video.mp4",
			Description:      &description,
		}

		input, err := dto.ToCreateVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, uploaderID, input.UploadUserID)
		assert.Equal(t, "Test Video", input.Title)
		assert.Equal(t, "s3://bucket/video.mp4", input.RawFileReference)
		require.NotNil(t, input.Description)
		assert.Equal(t, description, *input.Description)
	})

	t.Run("valid request without optional description", func(t *testing.T) {
		uploaderID := uuid.New()

		req := &videov1.CreateVideoRequest{
			UploadUserId:     uploaderID.String(),
			Title:            "Test Video",
			RawFileReference: "s3://bucket/video.mp4",
			Description:      nil,
		}

		input, err := dto.ToCreateVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, uploaderID, input.UploadUserID)
		assert.Equal(t, "Test Video", input.Title)
		assert.Equal(t, "s3://bucket/video.mp4", input.RawFileReference)
		assert.Nil(t, input.Description)
	})

	t.Run("invalid uploader UUID", func(t *testing.T) {
		req := &videov1.CreateVideoRequest{
			UploadUserId:     "not-a-valid-uuid",
			Title:            "Test Video",
			RawFileReference: "s3://bucket/video.mp4",
		}

		input, err := dto.ToCreateVideoInput(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid upload_user_id")
		assert.Equal(t, services.CreateVideoInput{}, input)
	})

	t.Run("empty uploader UUID", func(t *testing.T) {
		req := &videov1.CreateVideoRequest{
			UploadUserId:     "",
			Title:            "Test Video",
			RawFileReference: "s3://bucket/video.mp4",
		}

		input, err := dto.ToCreateVideoInput(req)

		assert.Error(t, err)
		assert.Equal(t, services.CreateVideoInput{}, input)
	})
}

func TestToUpdateVideoInput(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		videoID := uuid.New()
		title := "Updated Title"
		description := "Updated description"
		status := "published"
		mediaStatus := "completed"
		analysisStatus := "in_progress"
		durationMicros := int64(120000000)
		thumbnailURL := "https://cdn.example.com/thumb.jpg"
		hlsPlaylist := "https://cdn.example.com/master.m3u8"
		difficulty := "intermediate"
		summary := "Video summary"
		subtitleURL := "https://cdn.example.com/subtitle.vtt"
		errorMsg := "some error"

		req := &videov1.UpdateVideoRequest{
			VideoId:           videoID.String(),
			Title:             &title,
			Description:       &description,
			Status:            &status,
			MediaStatus:       &mediaStatus,
			AnalysisStatus:    &analysisStatus,
			DurationMicros:    &durationMicros,
			ThumbnailUrl:      &thumbnailURL,
			HlsMasterPlaylist: &hlsPlaylist,
			Difficulty:        &difficulty,
			Summary:           &summary,
			RawSubtitleUrl:    &subtitleURL,
			ErrorMessage:      &errorMsg,
		}

		input, err := dto.ToUpdateVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, videoID, input.VideoID)
		require.NotNil(t, input.Title)
		assert.Equal(t, title, *input.Title)
		require.NotNil(t, input.Description)
		assert.Equal(t, description, *input.Description)
		require.NotNil(t, input.Status)
		assert.Equal(t, status, *input.Status)
		require.NotNil(t, input.MediaStatus)
		assert.Equal(t, mediaStatus, *input.MediaStatus)
		require.NotNil(t, input.AnalysisStatus)
		assert.Equal(t, analysisStatus, *input.AnalysisStatus)
		require.NotNil(t, input.DurationMicros)
		assert.Equal(t, durationMicros, *input.DurationMicros)
		require.NotNil(t, input.ThumbnailURL)
		assert.Equal(t, thumbnailURL, *input.ThumbnailURL)
		require.NotNil(t, input.HLSMasterPlaylist)
		assert.Equal(t, hlsPlaylist, *input.HLSMasterPlaylist)
		require.NotNil(t, input.Difficulty)
		assert.Equal(t, difficulty, *input.Difficulty)
		require.NotNil(t, input.Summary)
		assert.Equal(t, summary, *input.Summary)
		require.NotNil(t, input.RawSubtitleURL)
		assert.Equal(t, subtitleURL, *input.RawSubtitleURL)
		require.NotNil(t, input.ErrorMessage)
		assert.Equal(t, errorMsg, *input.ErrorMessage)
	})

	t.Run("valid request with no optional fields", func(t *testing.T) {
		videoID := uuid.New()

		req := &videov1.UpdateVideoRequest{
			VideoId: videoID.String(),
		}

		input, err := dto.ToUpdateVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, videoID, input.VideoID)
		assert.Nil(t, input.Title)
		assert.Nil(t, input.Description)
		assert.Nil(t, input.Status)
		assert.Nil(t, input.MediaStatus)
		assert.Nil(t, input.AnalysisStatus)
		assert.Nil(t, input.DurationMicros)
		assert.Nil(t, input.ThumbnailURL)
		assert.Nil(t, input.HLSMasterPlaylist)
		assert.Nil(t, input.Difficulty)
		assert.Nil(t, input.Summary)
		assert.Nil(t, input.RawSubtitleURL)
		assert.Nil(t, input.ErrorMessage)
	})

	t.Run("invalid video UUID", func(t *testing.T) {
		req := &videov1.UpdateVideoRequest{
			VideoId: "not-a-valid-uuid",
		}

		input, err := dto.ToUpdateVideoInput(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid video_id")
		assert.Equal(t, services.UpdateVideoInput{}, input)
	})
}

func TestToDeleteVideoInput(t *testing.T) {
	t.Run("valid request with optional reason", func(t *testing.T) {
		videoID := uuid.New()
		reason := "cleanup"

		req := &videov1.DeleteVideoRequest{
			VideoId: videoID.String(),
			Reason:  &reason,
		}

		input, err := dto.ToDeleteVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, videoID, input.VideoID)
		require.NotNil(t, input.Reason)
		assert.Equal(t, reason, *input.Reason)
	})

	t.Run("valid request without reason", func(t *testing.T) {
		videoID := uuid.New()

		req := &videov1.DeleteVideoRequest{
			VideoId: videoID.String(),
		}

		input, err := dto.ToDeleteVideoInput(req)

		require.NoError(t, err)
		assert.Equal(t, videoID, input.VideoID)
		assert.Nil(t, input.Reason)
	})

	t.Run("invalid video UUID", func(t *testing.T) {
		req := &videov1.DeleteVideoRequest{
			VideoId: "not-a-valid-uuid",
		}

		input, err := dto.ToDeleteVideoInput(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid video_id")
		assert.Equal(t, services.DeleteVideoInput{}, input)
	})
}
