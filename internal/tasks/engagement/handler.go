package engagement

import (
	"context"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// EventHandler 处理 Engagement Event，负责写入 catalog.video_user_engagements_projection。
type EventHandler struct {
	repo    videoUserStatesStore
	tx      txmanager.Manager
	log     *log.Helper
	metrics *metrics
}

// NewEventHandler 构造 Engagement Event 处理器。
func NewEventHandler(repo videoUserStatesStore, tx txmanager.Manager, logger log.Logger, metrics *metrics) *EventHandler {
	return &EventHandler{
		repo:    repo,
		tx:      tx,
		log:     log.NewHelper(logger),
		metrics: metrics,
	}
}

// Handle 将事件投影至用户状态表。
func (h *EventHandler) Handle(ctx context.Context, evt *Event) error {
	if evt == nil {
		return fmt.Errorf("engagement: nil event")
	}

	action, err := evt.resolveAction()
	if err != nil {
		return errors.BadRequest("invalid-action", err.Error())
	}
	kind, err := evt.resolveKind()
	if err != nil {
		return errors.BadRequest("invalid-kind", err.Error())
	}

	userID, err := uuid.Parse(evt.UserID)
	if err != nil {
		return errors.BadRequest("invalid-user-id", "invalid user_id")
	}
	videoID, err := uuid.Parse(evt.VideoID)
	if err != nil {
		return errors.BadRequest("invalid-video-id", "invalid video_id")
	}

	occurredAt := evt.OccurredAt.UTC()

	err = h.tx.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		state, repoErr := h.repo.Get(txCtx, sess, userID, videoID)
		if repoErr != nil {
			return repoErr
		}

		hasLiked := state != nil && state.HasLiked
		hasBookmarked := state != nil && state.HasBookmarked
		var likedAt *time.Time
		var bookmarkedAt *time.Time
		if state != nil {
			likedAt = cloneTime(state.LikedOccurredAt)
			bookmarkedAt = cloneTime(state.BookmarkedOccurredAt)
		}

		switch kind {
		case kindLike:
			if state != nil && state.LikedOccurredAt != nil && !occurredAt.After(state.LikedOccurredAt.UTC()) {
				h.log.WithContext(txCtx).Debugf("skip stale like event: user=%s video=%s", userID, videoID)
				return nil
			}
			hasLiked = action == actionAdded
			likedAt = &occurredAt
		case kindBookmark:
			if state != nil && state.BookmarkedOccurredAt != nil && !occurredAt.After(state.BookmarkedOccurredAt.UTC()) {
				h.log.WithContext(txCtx).Debugf("skip stale bookmark event: user=%s video=%s", userID, videoID)
				return nil
			}
			hasBookmarked = action == actionAdded
			bookmarkedAt = &occurredAt
		default:
			h.log.WithContext(txCtx).Warnf("skip unknown engagement type: type=%s user=%s video=%s", evt.EngagementType, userID, videoID)
			return nil
		}

		upsert := repositories.UpsertVideoUserStateInput{
			UserID:               userID,
			VideoID:              videoID,
			HasLiked:             hasLiked,
			HasBookmarked:        hasBookmarked,
			LikedOccurredAt:      likedAt,
			BookmarkedOccurredAt: bookmarkedAt,
		}
		return h.repo.Upsert(txCtx, sess, upsert)
	})
	if err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return err
	}

	if h.metrics != nil {
		h.metrics.recordSuccess(ctx, occurredAt, time.Now())
	}
	return nil
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copied := t.UTC()
	return &copied
}

// videoUserStatesStore 定义 Engagement Handler 所需的仓储接口。
type videoUserStatesStore interface {
	Get(ctx context.Context, sess txmanager.Session, userID, videoID uuid.UUID) (*po.VideoUserState, error)
	Upsert(ctx context.Context, sess txmanager.Session, input repositories.UpsertVideoUserStateInput) error
}

var _ videoUserStatesStore = (*repositories.VideoUserStatesRepository)(nil)
