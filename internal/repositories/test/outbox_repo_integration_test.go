package repositories_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/docker/go-connections/nat"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestOutboxRepositoryIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn, terminate := startPostgres(ctx, t)
	defer terminate()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	applyMigrations(ctx, t, pool)

	repo := repositories.NewOutboxRepository(pool, log.NewStdLogger(io.Discard), outboxcfg.Config{Schema: "catalog"})

	eventID := uuid.New()
	aggregateID := uuid.New()
	msg := repositories.OutboxMessage{
		EventID:       eventID,
		AggregateType: "video",
		AggregateID:   aggregateID,
		EventType:     "catalog.video.created",
		Payload:       []byte(`{"video_id":"` + aggregateID.String() + `"}`),
		Headers: map[string]string{
			"schema_version": "v1",
		},
		AvailableAt: time.Now().UTC(),
	}

	require.NoError(t, repo.Enqueue(ctx, nil, msg))

	count, err := repo.CountPending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	claimNow := time.Now().UTC()
	lockTTL := claimNow.Add(-time.Minute)
	lockToken := uuid.NewString()

	pending, err := repo.ClaimPending(ctx, claimNow, lockTTL, 8, lockToken)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	require.NotNil(t, pending[0].LockToken)
	require.Equal(t, lockToken, *pending[0].LockToken)
	require.Nil(t, pending[0].PublishedAt)
	require.Equal(t, int32(0), pending[0].DeliveryAttempts)

	nextTime := claimNow.Add(250 * time.Millisecond)
	require.NoError(t, repo.Reschedule(ctx, nil, eventID, lockToken, nextTime, "publish timeout"))

	count, err = repo.CountPending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	lockToken2 := uuid.NewString()
	pendingAfterRetry, err := repo.ClaimPending(ctx, nextTime.Add(time.Millisecond), lockTTL, 4, lockToken2)
	require.NoError(t, err)
	require.Len(t, pendingAfterRetry, 1)
	require.Equal(t, int32(1), pendingAfterRetry[0].DeliveryAttempts)
	require.NotNil(t, pendingAfterRetry[0].LockToken)
	require.Equal(t, lockToken2, *pendingAfterRetry[0].LockToken)

	publishedAt := time.Now().UTC()
	require.NoError(t, repo.MarkPublished(ctx, nil, eventID, lockToken2, publishedAt))

	count, err = repo.CountPending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func startPostgres(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_DB":       "catalog",
		},
		WaitingFor: wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
			return fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skip outbox repo integration test: cannot start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/catalog?sslmode=disable", host, port.Port())
	cleanup := func() {
		termCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = container.Terminate(termCtx)
	}
	return dsn, cleanup
}

func applyMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	paths := make([]string, 0, len(files))
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		paths = append(paths, filepath.Join(migrationsDir, f.Name()))
	}
	sort.Strings(paths)

	for _, path := range paths {
		sqlBytes, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		_, execErr := pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, execErr, "apply migration %s", filepath.Base(path))
	}
}
