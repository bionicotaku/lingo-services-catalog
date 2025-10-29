package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/lingo-services-catalog/internal/repositories/sqlc"

	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VideoUserStatesRepository 维护用户与视频互动状态的持久化操作。
// 表结构参见 catalog.video_user_engagements_projection，由 Engagement 投影消费者写入。
type VideoUserStatesRepository struct {
	db      *pgxpool.Pool
	queries *catalogsql.Queries
	log     *log.Helper
}

// NewVideoUserStatesRepository 构造仓储实例，供 Wire 注入。
func NewVideoUserStatesRepository(db *pgxpool.Pool, logger log.Logger) *VideoUserStatesRepository {
	return &VideoUserStatesRepository{
		db:      db,
		queries: catalogsql.New(db),
		log:     log.NewHelper(logger),
	}
}

// UpsertVideoUserStateInput 描述一次用户互动状态写入。
type UpsertVideoUserStateInput struct {
	UserID               uuid.UUID
	VideoID              uuid.UUID
	HasLiked             bool
	HasBookmarked        bool
	LikedOccurredAt      *time.Time
	BookmarkedOccurredAt *time.Time
}

// Upsert 插入或更新用户互动状态，幂等覆盖最新状态。
func (r *VideoUserStatesRepository) Upsert(ctx context.Context, sess txmanager.Session, input UpsertVideoUserStateInput) error {
	if input.UserID == uuid.Nil || input.VideoID == uuid.Nil {
		return fmt.Errorf("upsert video user state: nil identifiers")
	}

	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := mappers.BuildUpsertVideoUserStateParams(
		input.UserID,
		input.VideoID,
		input.HasLiked,
		input.HasBookmarked,
		input.LikedOccurredAt,
		input.BookmarkedOccurredAt,
	)

	if err := queries.UpsertVideoUserState(ctx, params); err != nil {
		r.log.WithContext(ctx).Errorf("upsert video_user_state failed: user=%s video=%s err=%v", input.UserID, input.VideoID, err)
		return fmt.Errorf("upsert video_user_state: %w", err)
	}
	return nil
}

// Delete 移除一条用户互动状态记录。
func (r *VideoUserStatesRepository) Delete(ctx context.Context, sess txmanager.Session, userID, videoID uuid.UUID) error {
	if userID == uuid.Nil || videoID == uuid.Nil {
		return fmt.Errorf("delete video user state: nil identifiers")
	}

	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	if err := queries.DeleteVideoUserState(ctx, catalogsql.DeleteVideoUserStateParams{
		UserID:  userID,
		VideoID: videoID,
	}); err != nil {
		r.log.WithContext(ctx).Errorf("delete video_user_state failed: user=%s video=%s err=%v", userID, videoID, err)
		return fmt.Errorf("delete video_user_state: %w", err)
	}
	return nil
}

// Get 返回用户互动状态，若不存在则返回 nil。
func (r *VideoUserStatesRepository) Get(ctx context.Context, sess txmanager.Session, userID, videoID uuid.UUID) (*po.VideoUserState, error) {
	if userID == uuid.Nil || videoID == uuid.Nil {
		return nil, fmt.Errorf("get video user state: nil identifiers")
	}

	queries := r.queries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	record, err := queries.GetVideoUserState(ctx, catalogsql.GetVideoUserStateParams{
		UserID:  userID,
		VideoID: videoID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		r.log.WithContext(ctx).Errorf("get video_user_state failed: user=%s video=%s err=%v", userID, videoID, err)
		return nil, fmt.Errorf("get video_user_state: %w", err)
	}

	updatedAt := record.UpdatedAt.Time
	if !record.UpdatedAt.Valid {
		updatedAt = time.Time{}
	}

	var likedAtPtr *time.Time
	if record.LikedOccurredAt.Valid {
		value := record.LikedOccurredAt.Time
		likedAtPtr = &value
	}
	var bookmarkedAtPtr *time.Time
	if record.BookmarkedOccurredAt.Valid {
		value := record.BookmarkedOccurredAt.Time
		bookmarkedAtPtr = &value
	}

	return &po.VideoUserState{
		UserID:               record.UserID,
		VideoID:              record.VideoID,
		HasLiked:             record.HasLiked,
		HasBookmarked:        record.HasBookmarked,
		LikedOccurredAt:      likedAtPtr,
		BookmarkedOccurredAt: bookmarkedAtPtr,
		UpdatedAt:            updatedAt,
	}, nil
}
