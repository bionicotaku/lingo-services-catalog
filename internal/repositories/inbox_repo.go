package repositories

import (
	"context"
	"time"

	outboxpkg "github.com/bionicotaku/lingo-utils/outbox"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/outbox/store"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InboxEvent 描述需要写入 inbox_events 的事件数据。
type InboxEvent = store.InboxMessage

// InboxRepository 维护 Inbox 写入/状态操作。
type InboxRepository struct {
	delegate *store.Repository
}

// NewInboxRepository constructs an InboxRepository backed by the given pool.
func NewInboxRepository(db *pgxpool.Pool, logger log.Logger, cfg outboxcfg.Config) *InboxRepository {
	storeRepo, err := outboxpkg.NewRepository(db, logger, outboxpkg.RepositoryOptions{Schema: cfg.Schema})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init inbox repository failed", "error", err)
		storeRepo = store.NewRepository(db, logger)
	}
	return &InboxRepository{delegate: storeRepo}
}

// Insert persists a new inbox event within the provided transaction session.
func (r *InboxRepository) Insert(ctx context.Context, sess txmanager.Session, event InboxEvent) error {
	return r.delegate.RecordInboxEvent(ctx, sess, event)
}

// MarkProcessed marks an inbox event as processed at the specified time.
func (r *InboxRepository) MarkProcessed(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, processedAt time.Time) error {
	return r.delegate.MarkInboxProcessed(ctx, sess, eventID, processedAt)
}

// RecordError records the latest delivery error message for an inbox event.
func (r *InboxRepository) RecordError(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lastErr string) error {
	return r.delegate.RecordInboxError(ctx, sess, eventID, lastErr)
}

// Shared exposes the underlying store repository for advanced usage.
func (r *InboxRepository) Shared() *store.Repository {
	return r.delegate
}
