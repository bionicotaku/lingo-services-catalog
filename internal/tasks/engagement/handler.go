package engagement

import (
	"context"
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// EventHandler 处理 Engagement Event，负责写入 catalog.video_user_states。
type EventHandler struct {
	repo    *repositories.VideoUserStatesRepository
	tx      txmanager.Manager
	log     *log.Helper
	metrics *metrics
}

// NewEventHandler 构造 Engagement Event 处理器。
func NewEventHandler(repo *repositories.VideoUserStatesRepository, tx txmanager.Manager, logger log.Logger, metrics *metrics) *EventHandler {
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

	userID, err := uuid.Parse(evt.UserID)
	if err != nil {
		return errors.BadRequest("invalid-user-id", "invalid user_id")
	}
	videoID, err := uuid.Parse(evt.VideoID)
	if err != nil {
		return errors.BadRequest("invalid-video-id", "invalid video_id")
	}

	err = h.tx.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		state, repoErr := h.repo.Get(txCtx, sess, userID, videoID)
		if repoErr != nil {
			return repoErr
		}
		if state != nil && !evt.OccurredAt.After(state.OccurredAt) {
			h.log.WithContext(txCtx).Debugf("skip stale engagement event: user=%s video=%s", userID, videoID)
			return nil
		}
		hasLiked := valueOrDefault(evt.HasLiked, state != nil && state.HasLiked)
		hasBookmarked := valueOrDefault(evt.HasBookmarked, state != nil && state.HasBookmarked)
		hasWatched := valueOrDefault(evt.HasWatched, state != nil && state.HasWatched)

		upsert := repositories.UpsertVideoUserStateInput{
			UserID:        userID,
			VideoID:       videoID,
			HasLiked:      hasLiked,
			HasBookmarked: hasBookmarked,
			HasWatched:    hasWatched,
			OccurredAt:    evt.OccurredAt,
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
		h.metrics.recordSuccess(ctx, evt.OccurredAt, time.Now())
	}
	return nil
}

func valueOrDefault(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}
