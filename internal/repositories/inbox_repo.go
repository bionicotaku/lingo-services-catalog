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

// InboxMessage 表示需要记录的外部事件。
type InboxMessage = store.InboxMessage

// InboxEvent 表示已记录的外部事件。
type InboxEvent = store.InboxEvent

// InboxRepository 封装共享 Inbox 仓储实现。
type InboxRepository struct {
	delegate *store.Repository
}

// NewInboxRepository 构建 Inbox 仓储，内部复用 lingo-utils/outbox 仓储。
func NewInboxRepository(db *pgxpool.Pool, logger log.Logger, cfg outboxcfg.Config) *InboxRepository {
	storeRepo, err := outboxpkg.NewRepository(db, logger, outboxpkg.RepositoryOptions{Schema: cfg.Schema})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init inbox repository failed", "error", err)
		return &InboxRepository{delegate: store.NewRepository(db, logger)}
	}
	return &InboxRepository{delegate: storeRepo}
}

// Insert 在事务内记录 Inbox 事件。
func (r *InboxRepository) Insert(ctx context.Context, sess txmanager.Session, event InboxMessage) error {
	return r.delegate.RecordInboxEvent(ctx, sess, event)
}

// MarkProcessed 标记事件处理成功。
func (r *InboxRepository) MarkProcessed(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, processedAt time.Time) error {
	return r.delegate.MarkInboxProcessed(ctx, sess, eventID, processedAt)
}

// RecordError 更新事件处理错误信息。
func (r *InboxRepository) RecordError(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lastErr string) error {
	return r.delegate.RecordInboxError(ctx, sess, eventID, lastErr)
}

// Shared 暴露底层共享仓储，供 inbox runner 使用。
func (r *InboxRepository) Shared() *store.Repository {
	return r.delegate
}
