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

func TestBuildCreateVideoParams(t *testing.T) {
	t.Run("with description", func(t *testing.T) {
		uploadUserID := uuid.New()
		title := "Test Video"
		rawFileReference := "s3://bucket/video.mp4"
		description := "Test description"

		params := mappers.BuildCreateVideoParams(uploadUserID, title, rawFileReference, &description)

		assert.Equal(t, uploadUserID, params.UploadUserID)
		assert.Equal(t, title, params.Title)
		assert.Equal(t, rawFileReference, params.RawFileReference)
		assert.True(t, params.Description.Valid)
		assert.Equal(t, description, params.Description.String)
	})

	t.Run("without description", func(t *testing.T) {
		uploadUserID := uuid.New()
		title := "Test Video"
		rawFileReference := "s3://bucket/video.mp4"

		params := mappers.BuildCreateVideoParams(uploadUserID, title, rawFileReference, nil)

		assert.Equal(t, uploadUserID, params.UploadUserID)
		assert.Equal(t, title, params.Title)
		assert.Equal(t, rawFileReference, params.RawFileReference)
		assert.False(t, params.Description.Valid)
	})
}

func TestBuildUpdateVideoParams(t *testing.T) {
	t.Run("update all fields", func(t *testing.T) {
		videoID := uuid.New()
		title := "Updated Title"
		description := "Updated description"
		thumbnailURL := "https://cdn.example.com/thumb.jpg"
		hlsMasterPlaylist := "https://cdn.example.com/master.m3u8"
		difficulty := "intermediate"
		summary := "Video summary"
		rawSubtitleURL := "https://cdn.example.com/subtitle.vtt"
		errorMessage := "some error"
		status := po.VideoStatusPublished
		mediaStatus := po.StageReady
		analysisStatus := po.StageProcessing
		durationMicros := int64(120000000)

		params := mappers.BuildUpdateVideoParams(
			videoID,
			&title, &description, &thumbnailURL, &hlsMasterPlaylist,
			&difficulty, &summary, &rawSubtitleURL, &errorMessage,
			&status, &mediaStatus, &analysisStatus,
			&durationMicros,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.True(t, params.Title.Valid)
		assert.Equal(t, title, params.Title.String)
		assert.True(t, params.Description.Valid)
		assert.Equal(t, description, params.Description.String)
		assert.True(t, params.Status.Valid)
		assert.True(t, params.MediaStatus.Valid)
		assert.True(t, params.AnalysisStatus.Valid)
		assert.True(t, params.DurationMicros.Valid)
		assert.Equal(t, durationMicros, params.DurationMicros.Int64)
		assert.True(t, params.ThumbnailUrl.Valid)
		assert.Equal(t, thumbnailURL, params.ThumbnailUrl.String)
		assert.True(t, params.HlsMasterPlaylist.Valid)
		assert.Equal(t, hlsMasterPlaylist, params.HlsMasterPlaylist.String)
		assert.True(t, params.Difficulty.Valid)
		assert.Equal(t, difficulty, params.Difficulty.String)
		assert.True(t, params.Summary.Valid)
		assert.Equal(t, summary, params.Summary.String)
		assert.True(t, params.RawSubtitleUrl.Valid)
		assert.Equal(t, rawSubtitleURL, params.RawSubtitleUrl.String)
		assert.True(t, params.ErrorMessage.Valid)
		assert.Equal(t, errorMessage, params.ErrorMessage.String)
	})

	t.Run("update no fields (all nil)", func(t *testing.T) {
		videoID := uuid.New()

		params := mappers.BuildUpdateVideoParams(
			videoID,
			nil, nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.False(t, params.Title.Valid)
		assert.False(t, params.Description.Valid)
		assert.False(t, params.Status.Valid)
		assert.False(t, params.MediaStatus.Valid)
		assert.False(t, params.AnalysisStatus.Valid)
		assert.False(t, params.DurationMicros.Valid)
		assert.False(t, params.ThumbnailUrl.Valid)
		assert.False(t, params.HlsMasterPlaylist.Valid)
		assert.False(t, params.Difficulty.Valid)
		assert.False(t, params.Summary.Valid)
		assert.False(t, params.RawSubtitleUrl.Valid)
		assert.False(t, params.ErrorMessage.Valid)
	})

	t.Run("partial update - only title", func(t *testing.T) {
		videoID := uuid.New()
		title := "Only Title Updated"

		params := mappers.BuildUpdateVideoParams(
			videoID,
			&title, nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.True(t, params.Title.Valid)
		assert.Equal(t, title, params.Title.String)
		assert.False(t, params.Description.Valid)
	})
}

func TestVideoFromCatalog(t *testing.T) {
	t.Run("video with all fields", func(t *testing.T) {
		now := time.Now().UTC()
		videoID := uuid.New()
		uploadUserID := uuid.New()

		catalogVideo := catalogsql.CatalogVideo{
			VideoID:           videoID,
			UploadUserID:      uploadUserID,
			CreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
			Title:             "Test Video",
			Description:       pgtype.Text{String: "Description", Valid: true},
			RawFileReference:  "s3://bucket/video.mp4",
			Status:            po.VideoStatusPublished,
			MediaStatus:       po.StageReady,
			AnalysisStatus:    po.StageProcessing,
			RawFileSize:       pgtype.Int8{Int64: 1024000, Valid: true},
			RawResolution:     pgtype.Text{String: "1920x1080", Valid: true},
			RawBitrate:        pgtype.Int4{Int32: 5000, Valid: true},
			DurationMicros:    pgtype.Int8{Int64: 120000000, Valid: true},
			EncodedResolution: pgtype.Text{String: "1280x720", Valid: true},
			EncodedBitrate:    pgtype.Int4{Int32: 3000, Valid: true},
			ThumbnailUrl:      pgtype.Text{String: "https://cdn.example.com/thumb.jpg", Valid: true},
			HlsMasterPlaylist: pgtype.Text{String: "https://cdn.example.com/master.m3u8", Valid: true},
			Difficulty:        pgtype.Text{String: "intermediate", Valid: true},
			Summary:           pgtype.Text{String: "Summary", Valid: true},
			Tags:              []string{"tag1", "tag2"},
			RawSubtitleUrl:    pgtype.Text{String: "https://cdn.example.com/subtitle.vtt", Valid: true},
			ErrorMessage:      pgtype.Text{String: "some error", Valid: true},
		}

		video := mappers.VideoFromCatalog(catalogVideo)

		require.NotNil(t, video)
		assert.Equal(t, videoID, video.VideoID)
		assert.Equal(t, uploadUserID, video.UploadUserID)
		assert.True(t, now.Equal(video.CreatedAt))
		assert.True(t, now.Equal(video.UpdatedAt))
		assert.Equal(t, "Test Video", video.Title)
		require.NotNil(t, video.Description)
		assert.Equal(t, "Description", *video.Description)
		assert.Equal(t, "s3://bucket/video.mp4", video.RawFileReference)
		assert.Equal(t, po.VideoStatusPublished, video.Status)
		assert.Equal(t, po.StageReady, video.MediaStatus)
		assert.Equal(t, po.StageProcessing, video.AnalysisStatus)
		require.NotNil(t, video.RawFileSize)
		assert.Equal(t, int64(1024000), *video.RawFileSize)
		require.NotNil(t, video.RawResolution)
		assert.Equal(t, "1920x1080", *video.RawResolution)
		require.NotNil(t, video.DurationMicros)
		assert.Equal(t, int64(120000000), *video.DurationMicros)
		require.NotNil(t, video.ThumbnailURL)
		assert.Equal(t, "https://cdn.example.com/thumb.jpg", *video.ThumbnailURL)
		assert.Equal(t, []string{"tag1", "tag2"}, video.Tags)
	})

	t.Run("video with nil optional fields", func(t *testing.T) {
		now := time.Now().UTC()
		videoID := uuid.New()
		uploadUserID := uuid.New()

		catalogVideo := catalogsql.CatalogVideo{
			VideoID:          videoID,
			UploadUserID:     uploadUserID,
			CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			Title:            "Test Video",
			Description:      pgtype.Text{Valid: false},
			RawFileReference: "s3://bucket/video.mp4",
			Status:           po.VideoStatusPendingUpload,
			MediaStatus:      po.StagePending,
			AnalysisStatus:   po.StagePending,
			Tags:             []string{},
		}

		video := mappers.VideoFromCatalog(catalogVideo)

		require.NotNil(t, video)
		assert.Equal(t, videoID, video.VideoID)
		assert.Nil(t, video.Description)
		assert.Nil(t, video.RawFileSize)
		assert.Nil(t, video.RawResolution)
		assert.Nil(t, video.DurationMicros)
		assert.Nil(t, video.ThumbnailURL)
		assert.Nil(t, video.HLSMasterPlaylist)
		assert.Empty(t, video.Tags)
	})
}

func TestVideoReadyViewFromFindRow(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()

	row := catalogsql.FindVideoByIDRow{
		VideoID:        videoID,
		Title:          "Test Video",
		Status:         po.VideoStatusPublished,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}

	view := mappers.VideoReadyViewFromFindRow(row)

	require.NotNil(t, view)
	assert.Equal(t, videoID, view.VideoID)
	assert.Equal(t, "Test Video", view.Title)
	assert.Equal(t, po.VideoStatusPublished, view.Status)
	assert.Equal(t, po.StageReady, view.MediaStatus)
	assert.Equal(t, po.StageReady, view.AnalysisStatus)
	assert.True(t, now.Equal(view.CreatedAt))
	assert.True(t, now.Equal(view.UpdatedAt))
}

func TestVideoReadyViewFromListRow(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()

	row := catalogsql.ListReadyVideosForTestRow{
		VideoID:        videoID,
		Title:          "Test Video",
		Status:         po.VideoStatusPublished,
		MediaStatus:    po.StageReady,
		AnalysisStatus: po.StageReady,
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}

	view := mappers.VideoReadyViewFromListRow(row)

	require.NotNil(t, view)
	assert.Equal(t, videoID, view.VideoID)
	assert.Equal(t, "Test Video", view.Title)
	assert.Equal(t, po.VideoStatusPublished, view.Status)
	assert.Equal(t, po.StageReady, view.MediaStatus)
	assert.Equal(t, po.StageReady, view.AnalysisStatus)
	assert.True(t, now.Equal(view.CreatedAt))
	assert.True(t, now.Equal(view.UpdatedAt))
}

func TestToPgText(t *testing.T) {
	t.Run("non-nil string", func(t *testing.T) {
		value := "test"
		result := mappers.ToPgText(&value)

		assert.True(t, result.Valid)
		assert.Equal(t, "test", result.String)
	})

	t.Run("nil string", func(t *testing.T) {
		result := mappers.ToPgText(nil)

		assert.False(t, result.Valid)
	})

	t.Run("empty string", func(t *testing.T) {
		value := ""
		result := mappers.ToPgText(&value)

		assert.True(t, result.Valid)
		assert.Equal(t, "", result.String)
	})
}

func TestToPgInt8(t *testing.T) {
	t.Run("non-nil int64", func(t *testing.T) {
		value := int64(12345)
		result := mappers.ToPgInt8(&value)

		assert.True(t, result.Valid)
		assert.Equal(t, int64(12345), result.Int64)
	})

	t.Run("nil int64", func(t *testing.T) {
		result := mappers.ToPgInt8(nil)

		assert.False(t, result.Valid)
	})

	t.Run("zero value", func(t *testing.T) {
		value := int64(0)
		result := mappers.ToPgInt8(&value)

		assert.True(t, result.Valid)
		assert.Equal(t, int64(0), result.Int64)
	})
}

func TestToPgInt4(t *testing.T) {
	t.Run("non-nil int32", func(t *testing.T) {
		value := int32(12345)
		result := mappers.ToPgInt4(&value)

		assert.True(t, result.Valid)
		assert.Equal(t, int32(12345), result.Int32)
	})

	t.Run("nil int32", func(t *testing.T) {
		result := mappers.ToPgInt4(nil)

		assert.False(t, result.Valid)
	})
}

func TestToNullVideoStatus(t *testing.T) {
	t.Run("non-nil status", func(t *testing.T) {
		status := po.VideoStatusPublished
		result := mappers.ToNullVideoStatus(&status)

		assert.True(t, result.Valid)
		assert.Equal(t, catalogsql.CatalogVideoStatus(po.VideoStatusPublished), result.CatalogVideoStatus)
	})

	t.Run("nil status", func(t *testing.T) {
		result := mappers.ToNullVideoStatus(nil)

		assert.False(t, result.Valid)
	})
}

func TestToNullStageStatus(t *testing.T) {
	t.Run("non-nil stage status", func(t *testing.T) {
		status := po.StageReady
		result := mappers.ToNullStageStatus(&status)

		assert.True(t, result.Valid)
		assert.Equal(t, catalogsql.CatalogStageStatus(po.StageReady), result.CatalogStageStatus)
	})

	t.Run("nil stage status", func(t *testing.T) {
		result := mappers.ToNullStageStatus(nil)

		assert.False(t, result.Valid)
	})
}
