package engagement_test

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
	profilev1 "github.com/bionicotaku/lingo-services-profile/api/profile/v1"
	"github.com/bionicotaku/lingo-utils/outbox/store"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEventHandlerProcessesTimeline(t *testing.T) {
	repo := newFakeVideoUserStatesRepository()
	handler := engagement.NewEventHandler(repo, fakeStatsRepo{}, log.NewStdLogger(io.Discard), nil)

	ctx := context.Background()
	sess := fakeSession{}

	userID := uuid.New()
	videoID := uuid.New()
	baseTime := time.Now().Add(-10 * time.Minute).UTC()

	likeEvt := marshalEvent(t, &profilev1.EngagementAddedEvent{
		EventId:      uuid.New().String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: profilev1.FavoriteType_FAVORITE_TYPE_LIKE,
		OccurredAt:   timestamppb.New(baseTime),
	})
	require.NoError(t, handler.Handle(ctx, sess, likeEvt, &store.InboxEvent{EventType: "profile.engagement.added"}))

	state, ok := repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasLiked)
	require.False(t, state.HasBookmarked)
	require.NotNil(t, state.LikedOccurredAt)
	require.Equal(t, baseTime, state.LikedOccurredAt.UTC())

	bookmarkEvt := marshalEvent(t, &profilev1.EngagementAddedEvent{
		EventId:      uuid.New().String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK,
		OccurredAt:   timestamppb.New(baseTime.Add(2 * time.Minute)),
	})
	require.NoError(t, handler.Handle(ctx, sess, bookmarkEvt, &store.InboxEvent{EventType: "profile.engagement.added"}))

	state, ok = repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasBookmarked)
	require.Equal(t, baseTime.Add(2*time.Minute), state.BookmarkedOccurredAt.UTC())

	// Stale like removal - should be ignored
	staleUnlike := marshalEvent(t, &profilev1.EngagementRemovedEvent{
		EventId:      uuid.New().String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: profilev1.FavoriteType_FAVORITE_TYPE_LIKE,
		OccurredAt:   timestamppb.New(baseTime.Add(-time.Minute)),
	})
	require.NoError(t, handler.Handle(ctx, sess, staleUnlike, &store.InboxEvent{EventType: "profile.engagement.removed"}))

	state, _ = repo.state(userID, videoID)
	require.True(t, state.HasLiked)

	removeBookmark := marshalEvent(t, &profilev1.EngagementRemovedEvent{
		EventId:      uuid.New().String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: profilev1.FavoriteType_FAVORITE_TYPE_BOOKMARK,
		OccurredAt:   timestamppb.New(baseTime.Add(4 * time.Minute)),
	})
	require.NoError(t, handler.Handle(ctx, sess, removeBookmark, &store.InboxEvent{EventType: "profile.engagement.removed"}))

	state, _ = repo.state(userID, videoID)
	require.True(t, state.HasLiked)
	require.False(t, state.HasBookmarked)
	require.Equal(t, baseTime.Add(4*time.Minute), state.BookmarkedOccurredAt.UTC())
}

func TestEventHandlerInvalidFavoriteType(t *testing.T) {
	repo := newFakeVideoUserStatesRepository()
	handler := engagement.NewEventHandler(repo, fakeStatsRepo{}, log.NewStdLogger(io.Discard), nil)

	userID := uuid.New()
	videoID := uuid.New()
	evt := marshalEvent(t, &profilev1.EngagementAddedEvent{
		EventId:      uuid.New().String(),
		UserId:       userID.String(),
		VideoId:      videoID.String(),
		FavoriteType: profilev1.FavoriteType_FAVORITE_TYPE_UNSPECIFIED,
		OccurredAt:   timestamppb.Now(),
	})
	err := handler.Handle(context.Background(), fakeSession{}, evt, &store.InboxEvent{EventType: "profile.engagement.added"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid-favorite-type")

	_, ok := repo.state(userID, videoID)
	require.False(t, ok)
}

// ---- Test Doubles ----

type fakeVideoUserStatesRepository struct {
	mu     sync.Mutex
	states map[string]po.VideoUserState
}

func newFakeVideoUserStatesRepository() *fakeVideoUserStatesRepository {
	return &fakeVideoUserStatesRepository{states: make(map[string]po.VideoUserState)}
}

func (f *fakeVideoUserStatesRepository) Get(_ context.Context, _ txmanager.Session, userID, videoID uuid.UUID) (*po.VideoUserState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[stateKey(userID, videoID)]
	if !ok {
		return nil, nil
	}
	return cloneState(state), nil
}

func (f *fakeVideoUserStatesRepository) Upsert(_ context.Context, _ txmanager.Session, input repositories.UpsertVideoUserStateInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[stateKey(input.UserID, input.VideoID)] = po.VideoUserState{
		UserID:               input.UserID,
		VideoID:              input.VideoID,
		HasLiked:             input.HasLiked,
		HasBookmarked:        input.HasBookmarked,
		LikedOccurredAt:      cloneTime(input.LikedOccurredAt),
		BookmarkedOccurredAt: cloneTime(input.BookmarkedOccurredAt),
		UpdatedAt:            time.Now().UTC(),
	}
	return nil
}

func (f *fakeVideoUserStatesRepository) state(userID, videoID uuid.UUID) (po.VideoUserState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[stateKey(userID, videoID)]
	if !ok {
		return po.VideoUserState{}, false
	}
	return *cloneState(state), true
}

func stateKey(userID, videoID uuid.UUID) string {
	return userID.String() + "|" + videoID.String()
}

func cloneState(src po.VideoUserState) *po.VideoUserState {
	return &po.VideoUserState{
		UserID:               src.UserID,
		VideoID:              src.VideoID,
		HasLiked:             src.HasLiked,
		HasBookmarked:        src.HasBookmarked,
		LikedOccurredAt:      cloneTime(src.LikedOccurredAt),
		BookmarkedOccurredAt: cloneTime(src.BookmarkedOccurredAt),
		UpdatedAt:            src.UpdatedAt,
	}
}

func cloneTime(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	value := src.UTC()
	return &value
}

type fakeSession struct{}

func (fakeSession) Tx() pgx.Tx { return nil }

func (fakeSession) Context() context.Context { return context.Background() }

type fakeStatsRepo struct{}

func (fakeStatsRepo) Increment(context.Context, txmanager.Session, uuid.UUID, repositories.StatsDelta) (*po.VideoEngagementStatsProjection, error) {
	return nil, nil
}

func (fakeStatsRepo) MarkWatcher(context.Context, txmanager.Session, uuid.UUID, uuid.UUID, time.Time) (*po.VideoWatcherRecord, error) {
	return nil, nil
}

func marshalEvent(t *testing.T, msg proto.Message) *engagement.Event {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	return &engagement.Event{Payload: data}
}
