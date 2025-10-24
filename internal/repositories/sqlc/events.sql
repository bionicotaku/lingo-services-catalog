-- Outbox / Inbox 相关 SQL 定义

-- ============================================
-- Outbox 相关查询
-- ============================================

-- name: InsertOutboxEvent :one
INSERT INTO catalog.outbox_events (
    event_id,
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    headers,
    available_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
RETURNING
    event_id,
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    headers,
    occurred_at,
    available_at,
    published_at,
    delivery_attempts,
    last_error;

-- name: ClaimPendingOutboxEvents :many
SELECT
    event_id,
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    headers,
    occurred_at,
    available_at,
    published_at,
    delivery_attempts,
    last_error
FROM catalog.outbox_events
WHERE published_at IS NULL
  AND available_at <= $1
ORDER BY available_at
FOR UPDATE SKIP LOCKED
LIMIT $2;

-- name: MarkOutboxEventPublished :exec
UPDATE catalog.outbox_events
SET published_at = $2,
    delivery_attempts = delivery_attempts + 1,
    last_error = NULL
WHERE event_id = $1;

-- name: RescheduleOutboxEvent :exec
UPDATE catalog.outbox_events
SET delivery_attempts = delivery_attempts + 1,
    last_error = $2,
    available_at = $3
WHERE event_id = $1;

-- ============================================
-- Inbox 相关查询
-- ============================================

-- name: InsertInboxEvent :exec
INSERT INTO catalog.inbox_events (
    event_id,
    source_service,
    event_type,
    aggregate_type,
    aggregate_id,
    payload
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
ON CONFLICT (event_id) DO NOTHING;

-- name: MarkInboxEventProcessed :exec
UPDATE catalog.inbox_events
SET processed_at = $2,
    last_error = NULL
WHERE event_id = $1;

-- name: RecordInboxEventError :exec
UPDATE catalog.inbox_events
SET last_error = $2,
    processed_at = NULL
WHERE event_id = $1;

-- name: GetInboxEvent :one
SELECT
    event_id,
    source_service,
    event_type,
    aggregate_type,
    aggregate_id,
    payload,
    received_at,
    processed_at,
    last_error
FROM catalog.inbox_events
WHERE event_id = $1;
