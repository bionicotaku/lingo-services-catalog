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

// OutboxMessage 描述需要写入 outbox_events 的事件数据。
type OutboxMessage = store.Message

// OutboxEvent 表示从数据库读取的待发布事件。
type OutboxEvent = store.Event

// OutboxRepository 封装共享仓储实现，维持原有依赖注入接口。
type OutboxRepository struct {
	delegate *store.Repository
}

// NewOutboxRepository 构建 Outbox 仓储，内部复用 lingo-utils/outbox/repository。
func NewOutboxRepository(db *pgxpool.Pool, logger log.Logger, cfg outboxcfg.Config) *OutboxRepository {
	storeRepo, err := outboxpkg.NewRepository(db, logger, outboxpkg.RepositoryOptions{Schema: cfg.Schema})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init outbox repository failed", "error", err)
		return &OutboxRepository{delegate: store.NewRepository(db, logger)}
	}
	return &OutboxRepository{delegate: storeRepo}
}

// Enqueue 在事务内插入 Outbox 事件。
func (r *OutboxRepository) Enqueue(ctx context.Context, sess txmanager.Session, msg OutboxMessage) error {
	return r.delegate.Enqueue(ctx, sess, msg)
}

// ClaimPending 返回一批待发布的 Outbox 事件。
func (r *OutboxRepository) ClaimPending(ctx context.Context, availableBefore, staleBefore time.Time, limit int, lockToken string) ([]OutboxEvent, error) {
	return r.delegate.ClaimPending(ctx, availableBefore, staleBefore, limit, lockToken)
}

// MarkPublished 更新事件状态为已发布。
func (r *OutboxRepository) MarkPublished(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lockToken string, publishedAt time.Time) error {
	return r.delegate.MarkPublished(ctx, sess, eventID, lockToken, publishedAt)
}

// Reschedule 将事件重新安排在未来时间发布，并记录错误信息。
func (r *OutboxRepository) Reschedule(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lockToken string, nextAvailable time.Time, lastErr string) error {
	return r.delegate.Reschedule(ctx, sess, eventID, lockToken, nextAvailable, lastErr)
}

// CountPending 返回当前未发布的 Outbox 事件数量。
func (r *OutboxRepository) CountPending(ctx context.Context) (int64, error) {
	return r.delegate.CountPending(ctx)
}

// Shared 返回底层通用实现，供共享任务使用。
func (r *OutboxRepository) Shared() *store.Repository {
	return r.delegate
}
