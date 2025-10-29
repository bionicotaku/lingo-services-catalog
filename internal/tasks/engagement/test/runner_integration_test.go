package engagement_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRunnerProcessesProtoAndJSONPayloads(t *testing.T) {
	t.Parallel()

	repo := newFakeVideoUserStatesRepository()
	tx := &fakeTxManager{}
	subscriber := newControllableSubscriber(8)
	logger := log.NewStdLogger(io.Discard)

	runner, err := engagement.NewRunner(engagement.RunnerParams{
		Subscriber: subscriber,
		Repository: repo,
		TxManager:  tx,
		Logger:     logger,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	errorCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		errorCh <- runner.Run(ctx)
	}()

	userID := uuid.New()
	videoID := uuid.New()
	baseTime := time.Now().Add(-10 * time.Minute).UTC()

	protoPayload, err := proto.Marshal(&engagement.EventProto{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "like",
		UserId:         userID.String(),
		VideoId:        videoID.String(),
		OccurredAt:     timestamppb.New(baseTime),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: protoPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		return ok && state.HasLiked && state.LikedOccurredAt != nil && state.LikedOccurredAt.UTC().Equal(baseTime)
	}, time.Second, 20*time.Millisecond)

	jsonPayload, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(2 * time.Minute),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: jsonPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		if !ok {
			return false
		}
		return state.HasLiked && state.HasBookmarked &&
			state.LikedOccurredAt != nil && state.LikedOccurredAt.UTC().Equal(baseTime) &&
			state.BookmarkedOccurredAt != nil && state.BookmarkedOccurredAt.UTC().Equal(baseTime.Add(2*time.Minute))
	}, time.Second, 20*time.Millisecond)

	stalePayload, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.removed",
		State:          "removed",
		EngagementType: "like",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(-5 * time.Minute),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: stalePayload})

	removeBookmark, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.removed",
		State:          "removed",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     baseTime.Add(4 * time.Minute),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: removeBookmark})

	time.Sleep(50 * time.Millisecond)
	state, ok := repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasLiked)
	require.False(t, state.HasBookmarked)
	if state.LikedOccurredAt == nil {
		t.Fatalf("liked timestamp missing")
	}
	if state.BookmarkedOccurredAt == nil {
		t.Fatalf("bookmark timestamp missing")
	}
	require.Equal(t, baseTime, state.LikedOccurredAt.UTC())
	require.Equal(t, baseTime.Add(4*time.Minute), state.BookmarkedOccurredAt.UTC())
	require.Equal(t, 4, subscriber.Delivered())
	require.Equal(t, 4, tx.calls())

	subscriber.Close()
	cancel()
	select {
	case runErr := <-errorCh:
		if !errors.Is(runErr, context.Canceled) {
			require.NoError(t, runErr)
		}
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop")
	}
	wg.Wait()
}

func TestRunnerSkipsInvalidPayloadAndContinues(t *testing.T) {
	t.Parallel()

	repo := newFakeVideoUserStatesRepository()
	tx := &fakeTxManager{}
	subscriber := newControllableSubscriber(4)
	logger := log.NewStdLogger(io.Discard)

	runner, err := engagement.NewRunner(engagement.RunnerParams{
		Subscriber: subscriber,
		Repository: repo,
		TxManager:  tx,
		Logger:     logger,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	errorCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		errorCh <- runner.Run(ctx)
	}()

	subscriber.Publish(&gcpubsub.Message{Data: []byte("not-valid")})

	userID := uuid.New()
	videoID := uuid.New()
	validPayload, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "bookmark",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     time.Now().UTC(),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: validPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		return ok && state.HasBookmarked && state.BookmarkedOccurredAt != nil
	}, time.Second, 20*time.Millisecond)

	require.Equal(t, 1, repo.upsertCount())
	require.Equal(t, 1, tx.calls())

	subscriber.Close()
	cancel()
	select {
	case runErr := <-errorCh:
		if !errors.Is(runErr, context.Canceled) {
			require.NoError(t, runErr)
		}
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop")
	}
	wg.Wait()
}

func TestRunnerPropagatesHandlerError(t *testing.T) {
	t.Parallel()

	repo := newFakeVideoUserStatesRepository()
	tx := &fakeTxManager{}
	tx.setError(errors.New("tx failed"))
	subscriber := newControllableSubscriber(1)
	logger := log.NewStdLogger(io.Discard)

	runner, err := engagement.NewRunner(engagement.RunnerParams{
		Subscriber: subscriber,
		Repository: repo,
		TxManager:  tx,
		Logger:     logger,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errorCh := make(chan error, 1)
	go func() {
		errorCh <- runner.Run(ctx)
	}()

	userID := uuid.New()
	videoID := uuid.New()
	jsonPayload, err := json.Marshal(engagement.Event{
		EventName:      "profile.engagement.added",
		State:          "added",
		EngagementType: "like",
		UserID:         userID.String(),
		VideoID:        videoID.String(),
		OccurredAt:     time.Now().UTC(),
		Version:        engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: jsonPayload})
	subscriber.Close()

	select {
	case runErr := <-errorCh:
		require.Error(t, runErr)
		require.ErrorContains(t, runErr, "tx failed")
	case <-time.After(time.Second):
		t.Fatalf("runner did not return after handler error")
	}

	_, ok := repo.state(userID, videoID)
	require.False(t, ok)
	require.Equal(t, 1, tx.calls())
}

// ---- Test Doubles ----

type fakeVideoUserStatesRepository struct {
	mu     sync.Mutex
	states map[string]po.VideoUserState
	count  int
}

func newFakeVideoUserStatesRepository() *fakeVideoUserStatesRepository {
	return &fakeVideoUserStatesRepository{
		states: make(map[string]po.VideoUserState),
	}
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
	f.count++
	f.states[stateKey(input.UserID, input.VideoID)] = po.VideoUserState{
		UserID:               input.UserID,
		VideoID:              input.VideoID,
		HasLiked:             input.HasLiked,
		HasBookmarked:        input.HasBookmarked,
		LikedOccurredAt:      cloneTimePtr(input.LikedOccurredAt),
		BookmarkedOccurredAt: cloneTimePtr(input.BookmarkedOccurredAt),
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

func (f *fakeVideoUserStatesRepository) upsertCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
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
		LikedOccurredAt:      cloneTimePtr(src.LikedOccurredAt),
		BookmarkedOccurredAt: cloneTimePtr(src.BookmarkedOccurredAt),
		UpdatedAt:            src.UpdatedAt,
	}
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	value := t.UTC()
	return &value
}

type fakeTxManager struct {
	mu   sync.Mutex
	err  error
	call int
}

func (f *fakeTxManager) WithinTx(ctx context.Context, _ txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	f.mu.Lock()
	f.call++
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn == nil {
		return nil
	}
	return fn(ctx, fakeSession{})
}

func (f *fakeTxManager) WithinReadOnlyTx(ctx context.Context, opts txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	return f.WithinTx(ctx, opts, fn)
}

func (f *fakeTxManager) setError(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f *fakeTxManager) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call
}

type fakeSession struct{}

func (fakeSession) Tx() pgx.Tx { return nil }

func (fakeSession) Context() context.Context { return context.Background() }

type controllableSubscriber struct {
	ch        chan *gcpubsub.Message
	once      sync.Once
	mu        sync.Mutex
	delivered int
}

func newControllableSubscriber(buffer int) *controllableSubscriber {
	return &controllableSubscriber{ch: make(chan *gcpubsub.Message, buffer)}
}

func (s *controllableSubscriber) Publish(msg *gcpubsub.Message) {
	s.ch <- msg
}

func (s *controllableSubscriber) Close() {
	s.once.Do(func() { close(s.ch) })
}

func (s *controllableSubscriber) Receive(ctx context.Context, handler func(context.Context, *gcpubsub.Message) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-s.ch:
			if !ok {
				return nil
			}
			if msg == nil {
				continue
			}
			err := handler(ctx, msg)
			s.mu.Lock()
			s.delivered++
			s.mu.Unlock()
			if err != nil {
				return err
			}
		}
	}
}

func (s *controllableSubscriber) Stop() {
	s.Close()
}

func (s *controllableSubscriber) Delivered() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delivered
}
