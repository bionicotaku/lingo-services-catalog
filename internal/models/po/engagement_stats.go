package po

import (
	"time"

	"github.com/google/uuid"
)

// VideoEngagementStatsProjection 表示 catalog.video_engagement_stats_projection 记录。
type VideoEngagementStatsProjection struct {
	VideoID        uuid.UUID
	LikeCount      int64
	BookmarkCount  int64
	WatchCount     int64
	UniqueWatchers int64
	FirstWatchAt   *time.Time
	LastWatchAt    *time.Time
	UpdatedAt      time.Time
}

// VideoWatcherRecord 记录已计入 unique_watchers 的用户。
type VideoWatcherRecord struct {
	VideoID        uuid.UUID
	UserID         uuid.UUID
	FirstWatchedAt time.Time
	LastWatchedAt  time.Time
	Inserted       bool
}
