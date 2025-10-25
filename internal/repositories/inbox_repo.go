package repositories

import (
	"context"
	"time"

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

func NewInboxRepository(db *pgxpool.Pool, logger log.Logger) *InboxRepository {
	return &InboxRepository{delegate: store.NewRepository(db, logger)}
}

func (r *InboxRepository) Insert(ctx context.Context, sess txmanager.Session, event InboxEvent) error {
	return r.delegate.RecordInboxEvent(ctx, sess, event)
}

func (r *InboxRepository) MarkProcessed(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, processedAt time.Time) error {
	return r.delegate.MarkInboxProcessed(ctx, sess, eventID, processedAt)
}

func (r *InboxRepository) RecordError(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lastErr string) error {
	return r.delegate.RecordInboxError(ctx, sess, eventID, lastErr)
}

func (r *InboxRepository) Shared() *store.Repository {
	return r.delegate
}
