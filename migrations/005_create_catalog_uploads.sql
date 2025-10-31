-- ============================================
-- 5) 上传会话表：catalog.uploads
-- ============================================
create table if not exists catalog.uploads (
  video_id              uuid primary key,
  user_id               uuid not null,
  bucket                text not null,
  object_name           text not null,
  content_type          text,
  expected_size         bigint not null default 0,
  size_bytes            bigint not null default 0,
  content_md5           char(32) not null,
  title                 text not null,
  description           text not null,
  signed_url            text,
  signed_url_expires_at timestamptz,
  status                text not null check (status in ('uploading', 'completed', 'failed')),
  gcs_generation        text,
  gcs_etag              text,
  md5_hash              text,
  crc32c                text,
  error_code            text,
  error_message         text,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now()
);

comment on table catalog.uploads is '上传会话信息：记录预留 video_id、签名 URL 及回调校验结果';

comment on column catalog.uploads.video_id              is '预留的视频主键；OBJECT_FINALIZE 校验成功后用于创建 catalog.videos';
comment on column catalog.uploads.user_id               is '发起上传的用户 ID（auth.users.id）';
comment on column catalog.uploads.bucket                is '目标 GCS 存储桶名称';
comment on column catalog.uploads.object_name           is '对象路径（约定 raw_videos/{user_id}/{video_id}）';
comment on column catalog.uploads.content_type          is '客户端上报的 MIME 类型；用于白名单校验';
comment on column catalog.uploads.expected_size         is '客户端预估的文件大小（字节，0 表示未知）';
comment on column catalog.uploads.size_bytes            is '回调写入的实际文件大小（字节）';
comment on column catalog.uploads.content_md5           is '客户端计算的 MD5（hex 32），用于强唯一与回调对账';
comment on column catalog.uploads.title                 is '用户填写的视频标题';
comment on column catalog.uploads.description           is '用户填写的视频描述';
comment on column catalog.uploads.signed_url            is 'Resumable 会话初始化所用的 V4 Signed URL';
comment on column catalog.uploads.signed_url_expires_at is 'Signed URL 过期时间';
comment on column catalog.uploads.status                is '会话状态：uploading/completed/failed';
comment on column catalog.uploads.gcs_generation        is 'GCS generation；成功上传后用于幂等校验';
comment on column catalog.uploads.gcs_etag              is 'GCS ETag';
comment on column catalog.uploads.md5_hash              is 'OBJECT_FINALIZE payload 中的 md5Hash（hex 转换后）';
comment on column catalog.uploads.crc32c                is 'OBJECT_FINALIZE payload 中的 crc32c';
comment on column catalog.uploads.error_code            is '失败时记录的错误代码（如 MD5_MISMATCH）';
comment on column catalog.uploads.error_message         is '失败时的详细描述';
comment on column catalog.uploads.created_at            is '记录创建时间';
comment on column catalog.uploads.updated_at            is '最近更新时间；触发器自动维护';

create unique index if not exists uploads_user_md5_unique
  on catalog.uploads (user_id, content_md5);

comment on index catalog.uploads_user_md5_unique is '同一用户 + 内容哈希 的全局唯一约束';

create unique index if not exists uploads_bucket_object_unique
  on catalog.uploads (bucket, object_name);

comment on index catalog.uploads_bucket_object_unique is '同一 GCS 对象路径只允许一个上传会话';

do $$
begin
  if not exists (
    select 1 from pg_trigger where tgname = 'set_updated_at_on_uploads'
  ) then
    create trigger set_updated_at_on_uploads
      before update on catalog.uploads
      for each row execute function catalog.tg_set_updated_at();
  end if;
end$$;

comment on trigger set_updated_at_on_uploads on catalog.uploads
  is '更新 catalog.uploads 任意列时自动刷新 updated_at';
