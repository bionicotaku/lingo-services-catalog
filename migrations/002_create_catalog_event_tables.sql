-- ============================================
-- 6) Outbox 表：catalog.outbox_events
-- ============================================
create table if not exists catalog.outbox_events (
  event_id            uuid primary key default gen_random_uuid(),  -- 事件唯一标识
  aggregate_type      text not null,                               -- 聚合根类型，如 video
  aggregate_id        uuid not null,                               -- 聚合根主键（通常对应业务表主键）
  event_type          text not null,                               -- 领域事件名，如 catalog.video.ready
  payload             bytea not null,                              -- 事件负载
  headers             jsonb not null default '{}'::jsonb,          -- 追踪/幂等等头信息
  occurred_at         timestamptz not null default now(),          -- 事件产生时间
  available_at        timestamptz not null default now(),          -- 可发布时间（延迟投递时使用）
  published_at        timestamptz,                                 -- 发布成功时间
  delivery_attempts   integer not null default 0 check (delivery_attempts >= 0), -- 投递尝试次数
  last_error          text,                                        -- 最近一次失败原因
  lock_token          text,                                        -- 发布器租约标记
  locked_at           timestamptz                                  -- 租约获取时间
);

comment on table catalog.outbox_events is 'Outbox 表：与业务事务同库写入，后台扫描发布到事件总线';
comment on column catalog.outbox_events.aggregate_type    is '聚合根类型，限定于 catalog 服务内的实体（如 video）';
comment on column catalog.outbox_events.aggregate_id      is '聚合根主键，保持与业务表一致的 UUID';
comment on column catalog.outbox_events.event_type        is '事件名，使用过去式（如 catalog.video.ready）';
comment on column catalog.outbox_events.payload           is '事件负载（Protobuf 二进制），包含业务数据快照';
comment on column catalog.outbox_events.headers           is '事件头部（JSON），用于 trace/idempotency 等';
comment on column catalog.outbox_events.available_at      is '事件可被 Relay 选择的时间，支持延迟投递';
comment on column catalog.outbox_events.published_at      is '事件成功发布到消息通道的时间戳';
comment on column catalog.outbox_events.delivery_attempts is 'Outbox Relay 重试次数的累积值';
comment on column catalog.outbox_events.last_error        is '最近一次投递失败/异常的描述';
comment on column catalog.outbox_events.lock_token        is '发布器租约标记，标识由哪个实例认领';
comment on column catalog.outbox_events.locked_at         is '租约获取时间，防止长期占用';

create index if not exists outbox_events_available_idx
  on catalog.outbox_events (available_at)
  where published_at is null;
comment on index catalog.outbox_events_available_idx is '扫描未发布事件时按 available_at 排序';

create index if not exists outbox_events_lock_idx
  on catalog.outbox_events (lock_token)
  where lock_token is not null;
comment on index catalog.outbox_events_lock_idx is '租约查询，辅助排查是否存在长时间锁定的任务';

create index if not exists outbox_events_published_idx
  on catalog.outbox_events (published_at);
comment on index catalog.outbox_events_published_idx is '按发布状态过滤或审计事件';

-- ============================================
-- 7) Inbox 表：catalog.inbox_events
-- ============================================
create table if not exists catalog.inbox_events (
  event_id         uuid primary key,                     -- 来源事件唯一标识
  source_service   text not null,                        -- 事件来源服务，例如 media
  event_type       text not null,                        -- 事件名
  aggregate_type   text,                                 -- 来源聚合根类型
  aggregate_id     text,                                 -- 来源聚合根主键（文本以兼容多种类型）
  payload          bytea not null,                       -- 原始事件载荷快照
  received_at      timestamptz not null default now(),   -- 收到事件时间
  processed_at     timestamptz,                          -- 本服务处理完成时间
  last_error       text                                  -- 最近一次处理失败信息
);

comment on table catalog.inbox_events is 'Inbox 表：记录已消费的外部事件，保障处理幂等性';
comment on column catalog.inbox_events.event_id       is '来源事件的唯一标识，保证消费幂等';
comment on column catalog.inbox_events.source_service is '事件产生的服务上下文';
comment on column catalog.inbox_events.aggregate_type is '来源聚合根类型（可选，便于排查）';
comment on column catalog.inbox_events.aggregate_id   is '来源聚合根标识（文本化，兼容多类型主键）';
comment on column catalog.inbox_events.processed_at   is '事件处理成功的时间戳，NULL 表示仍待处理';

create index if not exists inbox_events_processed_idx
  on catalog.inbox_events (processed_at);
comment on index catalog.inbox_events_processed_idx is '按处理状态/时间过滤 Inbox 记录';
