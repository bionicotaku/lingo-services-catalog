package mappers_test

import (
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"
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

		visibilityStatus := "unlisted"
		now := time.Now().UTC()

		params := mappers.BuildCreateVideoParams(uploadUserID, title, rawFileReference, &description, &visibilityStatus, &now)

		assert.Equal(t, uploadUserID, params.UploadUserID)
		assert.Equal(t, title, params.Title)
		assert.Equal(t, rawFileReference, params.RawFileReference)
		assert.True(t, params.Description.Valid)
		assert.Equal(t, description, params.Description.String)
		assert.True(t, params.VisibilityStatus.Valid)
		assert.Equal(t, visibilityStatus, params.VisibilityStatus.String)
		assert.True(t, params.PublishAt.Valid)
		assert.WithinDuration(t, now, params.PublishAt.Time, time.Second)
	})

	t.Run("without description", func(t *testing.T) {
		uploadUserID := uuid.New()
		title := "Test Video"
		rawFileReference := "s3://bucket/video.mp4"

		params := mappers.BuildCreateVideoParams(uploadUserID, title, rawFileReference, nil, nil, nil)

		assert.Equal(t, uploadUserID, params.UploadUserID)
		assert.Equal(t, title, params.Title)
		assert.Equal(t, rawFileReference, params.RawFileReference)
		assert.False(t, params.Description.Valid)
		assert.False(t, params.VisibilityStatus.Valid)
		assert.False(t, params.PublishAt.Valid)
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
		rawFileSize := int64(5_000_000)
		rawResolution := "3840x2160"
		rawBitrate := int32(4200)
		tags := []string{"spoken", "lesson"}
		encodedResolution := "1920x1080"
		encodedBitrate := int32(3500)
		mediaJobID := "media-job-1"
		analysisJobID := "analysis-job-1"
		now := time.Now().UTC()
		visibilityStatus := "public"
		publishAt := now

		params := mappers.BuildUpdateVideoParams(
			videoID,
			&title, &description, &thumbnailURL, &hlsMasterPlaylist,
			&difficulty, &summary, &rawSubtitleURL, &errorMessage,
			&status, &mediaStatus, &analysisStatus,
			&rawFileSize, &rawResolution, &rawBitrate,
			&durationMicros,
			&encodedResolution,
			&encodedBitrate,
			&mediaJobID, &analysisJobID,
			&now, &now,
			tags,
			&visibilityStatus,
			&publishAt,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.True(t, params.Title.Valid)
		assert.Equal(t, title, params.Title.String)
		assert.True(t, params.Description.Valid)
		assert.Equal(t, description, params.Description.String)
		assert.True(t, params.Status.Valid)
		assert.True(t, params.MediaStatus.Valid)
		assert.True(t, params.AnalysisStatus.Valid)
		assert.True(t, params.RawFileSize.Valid)
		assert.Equal(t, rawFileSize, params.RawFileSize.Int64)
		assert.True(t, params.RawResolution.Valid)
		assert.Equal(t, rawResolution, params.RawResolution.String)
		assert.True(t, params.RawBitrate.Valid)
		assert.Equal(t, rawBitrate, params.RawBitrate.Int32)
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
		assert.True(t, params.MediaJobID.Valid)
		assert.Equal(t, mediaJobID, params.MediaJobID.String)
		assert.True(t, params.AnalysisJobID.Valid)
		assert.Equal(t, analysisJobID, params.AnalysisJobID.String)
		assert.True(t, params.MediaEmittedAt.Valid)
		assert.True(t, params.AnalysisEmittedAt.Valid)
		assert.ElementsMatch(t, tags, params.Tags)
		assert.True(t, params.VisibilityStatus.Valid)
		assert.Equal(t, visibilityStatus, params.VisibilityStatus.String)
		assert.True(t, params.PublishAt.Valid)
	})

	t.Run("update no fields (all nil)", func(t *testing.T) {
		videoID := uuid.New()
		var (
			status         *po.VideoStatus
			mediaStatus    *po.StageStatus
			analysisStatus *po.StageStatus
			emptyTags      []string
			strPtr         *string
			int64Ptr       *int64
			int32Ptr       *int32
			timePtr        *time.Time
		)

		params := mappers.BuildUpdateVideoParams(
			videoID,
			strPtr, strPtr, strPtr, strPtr,
			strPtr, strPtr, strPtr, strPtr,
			status,
			mediaStatus, analysisStatus,
			int64Ptr,
			strPtr,
			int32Ptr,
			int64Ptr,
			strPtr,
			int32Ptr,
			strPtr, strPtr,
			timePtr, timePtr,
			emptyTags,
			strPtr,
			timePtr,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.False(t, params.Title.Valid)
		assert.False(t, params.Description.Valid)
		assert.False(t, params.Status.Valid)
		assert.False(t, params.MediaStatus.Valid)
		assert.False(t, params.AnalysisStatus.Valid)
		assert.False(t, params.RawFileSize.Valid)
		assert.False(t, params.RawResolution.Valid)
		assert.False(t, params.RawBitrate.Valid)
		assert.False(t, params.DurationMicros.Valid)
		assert.False(t, params.EncodedResolution.Valid)
		assert.False(t, params.EncodedBitrate.Valid)
		assert.False(t, params.ThumbnailUrl.Valid)
		assert.False(t, params.HlsMasterPlaylist.Valid)
		assert.False(t, params.Difficulty.Valid)
		assert.False(t, params.Summary.Valid)
		assert.False(t, params.RawSubtitleUrl.Valid)
		assert.False(t, params.ErrorMessage.Valid)
		assert.Len(t, params.Tags, 0)
		assert.False(t, params.VisibilityStatus.Valid)
		assert.False(t, params.PublishAt.Valid)
	})

	t.Run("partial update - only title", func(t *testing.T) {
		videoID := uuid.New()
		title := "Only Title Updated"
		var (
			status         *po.VideoStatus
			mediaStatus    *po.StageStatus
			analysisStatus *po.StageStatus
			emptyTags      []string
			strPtr         *string
			int64Ptr       *int64
			int32Ptr       *int32
			timePtr        *time.Time
		)

		params := mappers.BuildUpdateVideoParams(
			videoID,
			&title, nil, nil, nil,
			strPtr, strPtr, strPtr, strPtr,
			status,
			mediaStatus, analysisStatus,
			int64Ptr,
			strPtr,
			int32Ptr,
			int64Ptr,
			strPtr,
			int32Ptr,
			strPtr, strPtr,
			timePtr, timePtr,
			emptyTags,
			strPtr,
			timePtr,
		)

		assert.Equal(t, videoID, params.VideoID)
		assert.True(t, params.Title.Valid)
		assert.Equal(t, title, params.Title.String)
		assert.False(t, params.Description.Valid)
		assert.False(t, params.EncodedResolution.Valid)
		assert.False(t, params.EncodedBitrate.Valid)
		assert.False(t, params.VisibilityStatus.Valid)
		assert.False(t, params.PublishAt.Valid)
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
			Version:           42,
			MediaStatus:       po.StageReady,
			AnalysisStatus:    po.StageProcessing,
			MediaJobID:        pgtype.Text{String: "media-job-1", Valid: true},
			MediaEmittedAt:    pgtype.Timestamptz{Time: now, Valid: true},
			AnalysisJobID:     pgtype.Text{String: "analysis-job-1", Valid: true},
			AnalysisEmittedAt: pgtype.Timestamptz{Time: now, Valid: true},
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
			VisibilityStatus:  "public",
			PublishAt:         pgtype.Timestamptz{Time: now, Valid: true},
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
		assert.Equal(t, int64(42), video.Version)
		require.NotNil(t, video.MediaJobID)
		assert.Equal(t, "media-job-1", *video.MediaJobID)
		require.NotNil(t, video.MediaEmittedAt)
		assert.True(t, now.Equal(*video.MediaEmittedAt))
		require.NotNil(t, video.AnalysisJobID)
		assert.Equal(t, "analysis-job-1", *video.AnalysisJobID)
		require.NotNil(t, video.AnalysisEmittedAt)
		assert.True(t, now.Equal(*video.AnalysisEmittedAt))
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
		assert.Equal(t, "public", video.VisibilityStatus)
		require.NotNil(t, video.PublishAt)
		assert.True(t, now.Equal(*video.PublishAt))
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
			Version:          1,
			MediaStatus:      po.StagePending,
			AnalysisStatus:   po.StagePending,
			Tags:             []string{},
			VisibilityStatus: "public",
		}

		video := mappers.VideoFromCatalog(catalogVideo)

		require.NotNil(t, video)
		assert.Equal(t, videoID, video.VideoID)
		assert.Nil(t, video.Description)
		assert.Equal(t, int64(1), video.Version)
		assert.Nil(t, video.MediaJobID)
		assert.Nil(t, video.MediaEmittedAt)
		assert.Nil(t, video.AnalysisJobID)
		assert.Nil(t, video.AnalysisEmittedAt)
		assert.Nil(t, video.RawFileSize)
		assert.Nil(t, video.RawResolution)
		assert.Nil(t, video.DurationMicros)
		assert.Nil(t, video.ThumbnailURL)
		assert.Nil(t, video.HLSMasterPlaylist)
		assert.Empty(t, video.Tags)
		assert.Equal(t, "public", video.VisibilityStatus)
		assert.Nil(t, video.PublishAt)
	})
}

func TestVideoReadyViewFromFindRow(t *testing.T) {
	now := time.Now().UTC()
	videoID := uuid.New()

	row := catalogsql.FindPublishedVideoRow{
		VideoID:          videoID,
		Title:            "Test Video",
		Status:           po.VideoStatusPublished,
		MediaStatus:      po.StageReady,
		AnalysisStatus:   po.StageReady,
		CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		VisibilityStatus: "public",
		PublishAt:        pgtype.Timestamptz{Time: now, Valid: true},
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
	assert.Equal(t, "public", view.VisibilityStatus)
	if assert.NotNil(t, view.PublishAt) {
		assert.True(t, now.Equal(*view.PublishAt))
	}
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
