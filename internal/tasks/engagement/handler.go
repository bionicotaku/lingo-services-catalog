package engagement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	profilev1 "github.com/bionicotaku/lingo-services-profile/api/profile/v1"
	"github.com/bionicotaku/lingo-utils/outbox/store"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EventHandler 处理 Engagement Event，负责写入 catalog.video_user_engagements_projection。
type EventHandler struct {
	repo    videoUserStatesStore
	stats   videoEngagementStatsStore
	log     *log.Helper
	metrics *metrics
}

// NewEventHandler 构造 Engagement Event 处理器。
func NewEventHandler(repo videoUserStatesStore, stats videoEngagementStatsStore, logger log.Logger, metrics *metrics) *EventHandler {
	return &EventHandler{
		repo:    repo,
		stats:   stats,
		log:     log.NewHelper(logger),
		metrics: metrics,
	}
}

// Handle 将事件投影至用户状态表。
func (h *EventHandler) Handle(ctx context.Context, sess txmanager.Session, evt *Event, inboxEvt *store.InboxEvent) error {
	if evt == nil || len(evt.Payload) == 0 {
		return fmt.Errorf("engagement: empty event payload")
	}
	if inboxEvt == nil {
		return fmt.Errorf("engagement: inbox event metadata missing")
	}

	eventType := strings.TrimSpace(inboxEvt.EventType)
	switch eventType {
	case "profile.engagement.added":
		return h.handleEngagementMutation(ctx, sess, evt.Payload, actionAdded)
	case "profile.engagement.removed":
		return h.handleEngagementMutation(ctx, sess, evt.Payload, actionRemoved)
	case "profile.watch.progressed":
		return h.handleWatchProgress(ctx, sess, evt.Payload)
	default:
		h.log.WithContext(ctx).Debugf("engagement: skip unsupported event type %s", eventType)
		return nil
	}
}

type actionType string

const (
	actionUnknown actionType = ""
	actionAdded   actionType = "added"
	actionRemoved actionType = "removed"
)

type engagementKind string

const (
	kindUnknown  engagementKind = ""
	kindLike     engagementKind = "like"
	kindBookmark engagementKind = "bookmark"
)

func (h *EventHandler) handleEngagementMutation(ctx context.Context, sess txmanager.Session, payload []byte, action actionType) error {
	if len(payload) == 0 {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return fmt.Errorf("engagement: empty payload for mutation")
	}

	switch action {
	case actionAdded:
		var msg profilev1.EngagementAddedEvent
		if err := proto.Unmarshal(payload, &msg); err != nil {
			if h.metrics != nil {
				h.metrics.recordFailure(ctx)
			}
			return fmt.Errorf("engagement: unmarshal added event: %w", err)
		}
		return h.applyEngagement(ctx, sess, msg.GetUserId(), msg.GetVideoId(), msg.GetFavoriteType(), msg.GetOccurredAt(), actionAdded)
	case actionRemoved:
		var msg profilev1.EngagementRemovedEvent
		if err := proto.Unmarshal(payload, &msg); err != nil {
			if h.metrics != nil {
				h.metrics.recordFailure(ctx)
			}
			return fmt.Errorf("engagement: unmarshal removed event: %w", err)
		}
		return h.applyEngagement(ctx, sess, msg.GetUserId(), msg.GetVideoId(), msg.GetFavoriteType(), msg.GetOccurredAt(), actionRemoved)
	default:
		return nil
	}
}

func (h *EventHandler) applyEngagement(ctx context.Context, sess txmanager.Session, userIDRaw, videoIDRaw string, favorite profilev1.FavoriteType, ts *timestamppb.Timestamp, action actionType) error {
	kind, err := convertFavoriteType(favorite)
	if err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return errors.BadRequest("invalid-favorite-type", err.Error())
	}

	userID, err := uuid.Parse(strings.TrimSpace(userIDRaw))
	if err != nil {
		return errors.BadRequest("invalid-user-id", "invalid user_id")
	}
	videoID, err := uuid.Parse(strings.TrimSpace(videoIDRaw))
	if err != nil {
		return errors.BadRequest("invalid-video-id", "invalid video_id")
	}

	occurredAt := timestampToTime(ts)

	state, repoErr := h.repo.Get(ctx, sess, userID, videoID)
	if repoErr != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return repoErr
	}

	hasLiked := state != nil && state.HasLiked
	hasBookmarked := state != nil && state.HasBookmarked
	prevLiked := hasLiked
	prevBookmarked := hasBookmarked
	var likeDelta int64
	var bookmarkDelta int64
	var likedAt *time.Time
	var bookmarkedAt *time.Time
	if state != nil {
		likedAt = cloneTime(state.LikedOccurredAt)
		bookmarkedAt = cloneTime(state.BookmarkedOccurredAt)
	}

	switch kind {
	case kindLike:
		if state != nil && state.LikedOccurredAt != nil && !occurredAt.After(state.LikedOccurredAt.UTC()) {
			h.log.WithContext(ctx).Debugf("skip stale like event: user=%s video=%s", userID, videoID)
			return nil
		}
		hasLiked = action == actionAdded
		likedAt = &occurredAt
		if hasLiked != prevLiked {
			if hasLiked {
				likeDelta = 1
			} else {
				likeDelta = -1
			}
		}
	case kindBookmark:
		if state != nil && state.BookmarkedOccurredAt != nil && !occurredAt.After(state.BookmarkedOccurredAt.UTC()) {
			h.log.WithContext(ctx).Debugf("skip stale bookmark event: user=%s video=%s", userID, videoID)
			return nil
		}
		hasBookmarked = action == actionAdded
		bookmarkedAt = &occurredAt
		if hasBookmarked != prevBookmarked {
			if hasBookmarked {
				bookmarkDelta = 1
			} else {
				bookmarkDelta = -1
			}
		}
	default:
		h.log.WithContext(ctx).Warnf("skip unknown favorite type: user=%s video=%s favorite=%v", userID, videoID, favorite)
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
	if err := h.repo.Upsert(ctx, sess, upsert); err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return err
	}

	if h.stats != nil && (likeDelta != 0 || bookmarkDelta != 0) {
		if _, err := h.stats.Increment(ctx, sess, videoID, repositories.StatsDelta{
			LikeDelta:     likeDelta,
			BookmarkDelta: bookmarkDelta,
		}); err != nil {
			if h.metrics != nil {
				h.metrics.recordFailure(ctx)
			}
			return err
		}
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

type videoEngagementStatsStore interface {
	Increment(ctx context.Context, sess txmanager.Session, videoID uuid.UUID, delta repositories.StatsDelta) (*po.VideoEngagementStatsProjection, error)
	MarkWatcher(ctx context.Context, sess txmanager.Session, videoID, userID uuid.UUID, watchTime time.Time) (*po.VideoWatcherRecord, error)
}

var _ videoEngagementStatsStore = (*repositories.VideoEngagementStatsRepository)(nil)

func (h *EventHandler) handleWatchProgress(ctx context.Context, sess txmanager.Session, payload []byte) error {
	if h.stats == nil {
		h.log.WithContext(ctx).Warn("engagement: stats repository not configured, skip watch.progressed event")
		return nil
	}

	var msg profilev1.WatchProgressedEvent
	if err := proto.Unmarshal(payload, &msg); err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return fmt.Errorf("engagement: unmarshal watch progressed: %w", err)
	}

	userID, err := uuid.Parse(strings.TrimSpace(msg.GetUserId()))
	if err != nil {
		return errors.BadRequest("invalid-user-id", "invalid user_id")
	}
	videoID, err := uuid.Parse(strings.TrimSpace(msg.GetVideoId()))
	if err != nil {
		return errors.BadRequest("invalid-video-id", "invalid video_id")
	}

	watchTime := time.Now().UTC()
	if progress := msg.GetProgress(); progress != nil {
		if ts := progress.GetLastWatchedAt(); ts != nil {
			watchTime = ts.AsTime().UTC()
		} else if ts := progress.GetFirstWatchedAt(); ts != nil {
			watchTime = ts.AsTime().UTC()
		}
	}

	record, err := h.stats.MarkWatcher(ctx, sess, videoID, userID, watchTime)
	if err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return err
	}

	uniqueDelta := int64(0)
	if record != nil && record.Inserted {
		uniqueDelta = 1
	}
	delta := repositories.StatsDelta{
		WatchDelta:         1,
		UniqueWatcherDelta: uniqueDelta,
		LastWatchAt:        &watchTime,
	}
	if uniqueDelta == 1 {
		delta.FirstWatchAt = &watchTime
	}

	if _, err := h.stats.Increment(ctx, sess, videoID, delta); err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx)
		}
		return err
	}

	if h.metrics != nil {
		h.metrics.recordSuccess(ctx, watchTime, time.Now())
	}
	return nil
}

func convertFavoriteType(ft profilev1.FavoriteType) (engagementKind, error) {
	switch ft {
	case profilev1.FavoriteType_FAVORITE_TYPE_LIKE:
		return kindLike, nil
	case profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK:
		return kindBookmark, nil
	default:
		return kindUnknown, fmt.Errorf("unsupported favorite_type=%v", ft)
	}
}

func timestampToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Now().UTC()
	}
	t := ts.AsTime().UTC()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
