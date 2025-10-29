package mappers

import (
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"
)

// VideoEngagementStatsFromRow 转换 sqlc 结果为投影视图。
func VideoEngagementStatsFromRow(row catalogsql.CatalogVideoEngagementStatsProjection) *po.VideoEngagementStatsProjection {
	return &po.VideoEngagementStatsProjection{
		VideoID:        row.VideoID,
		LikeCount:      row.LikeCount,
		BookmarkCount:  row.BookmarkCount,
		WatchCount:     row.WatchCount,
		UniqueWatchers: row.UniqueWatchers,
		FirstWatchAt:   timestampPtr(row.FirstWatchAt),
		LastWatchAt:    timestampPtr(row.LastWatchAt),
		UpdatedAt:      mustTimestamp(row.UpdatedAt),
	}
}

// VideoWatcherRecordFromRow 转换 watcher upsert 返回值。
func VideoWatcherRecordFromRow(row catalogsql.UpsertVideoWatcherRow) *po.VideoWatcherRecord {
	return &po.VideoWatcherRecord{
		VideoID:        row.VideoID,
		UserID:         row.UserID,
		FirstWatchedAt: mustTimestamp(row.FirstWatchedAt),
		LastWatchedAt:  mustTimestamp(row.LastWatchedAt),
		Inserted:       row.Inserted,
	}
}
