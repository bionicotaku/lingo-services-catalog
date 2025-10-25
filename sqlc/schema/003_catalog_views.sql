CREATE VIEW catalog.videos_ready_view AS
SELECT
  video_id,
  title,
  status,
  media_status,
  analysis_status,
  created_at,
  updated_at
FROM catalog.videos
WHERE status IN ('ready', 'published');
