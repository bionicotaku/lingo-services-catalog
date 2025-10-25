-- ============================================
-- 8) 测试只读视图：catalog.videos_ready_view
-- ============================================
create or replace view catalog.videos_ready_view as
select
  v.video_id,
  v.title,
  v.status,
  v.media_status,
  v.analysis_status,
  v.created_at,
  v.updated_at
from catalog.videos v
where v.status in ('ready', 'published');

comment on view catalog.videos_ready_view
  is '测试用只读视图：展示状态为 ready/published 的视频基础信息';
