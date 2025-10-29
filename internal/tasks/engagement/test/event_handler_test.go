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
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestEventHandlerProcessesTimeline(t *testing.T) {
	repo := newFakeVideoUserStatesRepository()
	handler := engagement.NewEventHandler(repo, log.NewStdLogger(io.Discard), nil)

	ctx := context.Background()
	sess := fakeSession{}

	userID := uuid.New()
	videoID := uuid.New()
	baseTime := time.Now().Add(-10 * time.Minute).UTC()

	likeEvt := &engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "like",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime,
		Version:        engagement.EventVersion,
	}
	require.NoError(t, handler.Handle(ctx, sess, likeEvt, nil))

	state, ok := repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasLiked)
	require.False(t, state.HasBookmarked)
	require.NotNil(t, state.LikedOccurredAt)
	require.Equal(t, baseTime, state.LikedOccurredAt.UTC())

	bookmarkEvt := &engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(2 * time.Minute),
		Version:        engagement.EventVersion,
	}
	require.NoError(t, handler.Handle(ctx, sess, bookmarkEvt, nil))

	state, ok = repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasBookmarked)
	require.Equal(t, baseTime.Add(2*time.Minute), state.BookmarkedOccurredAt.UTC())

	// Stale like removal - should be ignored
	staleUnlike := &engagement.Event{
		EventName:      "profile.engagement.removed",
		State:          "removed",
		EngagementType: "like",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(-time.Minute),
		Version:        engagement.EventVersion,
	}
	require.NoError(t, handler.Handle(ctx, sess, staleUnlike, nil))

	state, _ = repo.state(userID, videoID)
	require.True(t, state.HasLiked)

	removeBookmark := &engagement.Event{
		EventName:      "profile.engagement.removed",
		State:          "removed",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(4 * time.Minute),
		Version:        engagement.EventVersion,
	}
	require.NoError(t, handler.Handle(ctx, sess, removeBookmark, nil))

	state, _ = repo.state(userID, videoID)
	require.True(t, state.HasLiked)
	require.False(t, state.HasBookmarked)
	require.Equal(t, baseTime.Add(4*time.Minute), state.BookmarkedOccurredAt.UTC())
}

func TestEventHandlerUnknownTypeIsIgnored(t *testing.T) {
	repo := newFakeVideoUserStatesRepository()
	handler := engagement.NewEventHandler(repo, log.NewStdLogger(io.Discard), nil)

	userID := uuid.New()
	videoID := uuid.New()
	evt := &engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "comment",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     time.Now().UTC(),
		Version:        engagement.EventVersion,
	}
	err := handler.Handle(context.Background(), fakeSession{}, evt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported engagement_type")

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
