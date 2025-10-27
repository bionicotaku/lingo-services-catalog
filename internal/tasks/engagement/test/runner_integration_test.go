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

	liked := true
	protoPayload, err := proto.Marshal(&engagement.EventProto{
		UserId:     userID.String(),
		VideoId:    videoID.String(),
		HasLiked:   &liked,
		OccurredAt: timestamppb.New(baseTime),
		Version:    engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: protoPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		return ok && state.HasLiked && state.OccurredAt.Equal(baseTime)
	}, time.Second, 20*time.Millisecond)

	bookmark := true
	jsonPayload, err := json.Marshal(engagement.Event{
		UserID:        userID.String(),
		VideoID:       videoID.String(),
		HasBookmarked: &bookmark,
		OccurredAt:    baseTime.Add(2 * time.Minute),
		Version:       engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: jsonPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		if !ok {
			return false
		}
		return state.HasLiked && state.HasBookmarked && state.OccurredAt.Equal(baseTime.Add(2*time.Minute))
	}, time.Second, 20*time.Millisecond)

	staleLike := false
	stalePayload, err := json.Marshal(engagement.Event{
		UserID:     userID.String(),
		VideoID:    videoID.String(),
		HasLiked:   &staleLike,
		OccurredAt: baseTime.Add(-5 * time.Minute),
		Version:    engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: stalePayload})

	time.Sleep(50 * time.Millisecond)
	state, ok := repo.state(userID, videoID)
	require.True(t, ok)
	require.True(t, state.HasLiked)
	require.True(t, state.HasBookmarked)
	require.False(t, state.HasWatched)
	require.Equal(t, baseTime.Add(2*time.Minute), state.OccurredAt)
	require.Equal(t, 3, subscriber.Delivered())
	require.Equal(t, 3, tx.calls())

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
	watched := true
	validPayload, err := json.Marshal(engagement.Event{
		UserID:     userID.String(),
		VideoID:    videoID.String(),
		HasWatched: &watched,
		OccurredAt: time.Now().UTC(),
		Version:    engagement.EventVersion,
	})
	require.NoError(t, err)
	subscriber.Publish(&gcpubsub.Message{Data: validPayload})

	require.Eventually(t, func() bool {
		state, ok := repo.state(userID, videoID)
		return ok && state.HasWatched
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
		UserID:     userID.String(),
		VideoID:    videoID.String(),
		HasLiked:   ptrBool(true),
		OccurredAt: time.Now().UTC(),
		Version:    engagement.EventVersion,
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
	cloned := state
	return &cloned, nil
}

func (f *fakeVideoUserStatesRepository) Upsert(_ context.Context, _ txmanager.Session, input repositories.UpsertVideoUserStateInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	f.states[stateKey(input.UserID, input.VideoID)] = po.VideoUserState{
		UserID:        input.UserID,
		VideoID:       input.VideoID,
		HasLiked:      input.HasLiked,
		HasBookmarked: input.HasBookmarked,
		HasWatched:    input.HasWatched,
		OccurredAt:    input.OccurredAt,
		UpdatedAt:     time.Now().UTC(),
	}
	return nil
}

func (f *fakeVideoUserStatesRepository) state(userID, videoID uuid.UUID) (po.VideoUserState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[stateKey(userID, videoID)]
	return state, ok
}

func (f *fakeVideoUserStatesRepository) upsertCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func stateKey(userID, videoID uuid.UUID) string {
	return userID.String() + "|" + videoID.String()
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

func ptrBool(b bool) *bool {
	return &b
}
