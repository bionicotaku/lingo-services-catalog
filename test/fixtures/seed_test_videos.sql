-- ============================================
-- 测试数据：为 test@test.com 用户添加多样化的视频数据
-- ============================================
-- 测试用户ID
\set test_user_id 'f0ad5a16-0d50-4f94-8ff7-b99dda13ee47'

-- 1) pending_upload 状态（刚创建记录，等待上传完成）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status
) VALUES
  (:'test_user_id', 'Introduction to English Pronunciation',
   'Learn basic English pronunciation rules for beginners',
   'gs://learning-app-media/uploads/2025/intro_pronunciation_pending.mp4',
   'pending_upload', 'pending', 'pending'),

  (:'test_user_id', 'Daily Conversation Practice #1',
   'Practice common daily English conversations',
   'gs://learning-app-media/uploads/2025/conversation_01_pending.mp4',
   'pending_upload', 'pending', 'pending');

-- 2) processing 状态（正在转码/分析中）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate
) VALUES
  (:'test_user_id', 'English Grammar: Present Tense',
   'Comprehensive guide to present tense in English',
   'gs://learning-app-media/uploads/2025/grammar_present_tense.mp4',
   'processing', 'processing', 'pending',
   52428800, '1920x1080', 5000),

  (:'test_user_id', 'Business English Email Writing',
   'Professional email writing techniques for business communication',
   'gs://learning-app-media/uploads/2025/business_email.mp4',
   'processing', 'ready', 'processing',
   73400320, '1920x1080', 5500);

-- 3) ready 状态（已完成处理，待发布）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate,
  duration_micros, encoded_resolution, encoded_bitrate,
  thumbnail_url, hls_master_playlist,
  difficulty, summary, tags, raw_subtitle_url
) VALUES
  (:'test_user_id', 'IELTS Speaking Test Preparation',
   'Strategies and tips for IELTS speaking section',
   'gs://learning-app-media/uploads/2025/ielts_speaking.mp4',
   'ready', 'ready', 'ready',
   94371840, '1920x1080', 6000,
   1200000000, '1920x1080', 4500,
   'gs://learning-app-media/thumbnails/ielts_speaking_thumb.jpg',
   'gs://learning-app-media/hls/ielts_speaking/master.m3u8',
   'Advanced', 'Comprehensive preparation guide for IELTS speaking test with real examples',
   ARRAY['IELTS', 'Speaking', 'Test Preparation', 'Advanced'],
   'gs://learning-app-media/subtitles/ielts_speaking_en.vtt'),

  (:'test_user_id', 'Phrasal Verbs for Daily Use',
   'Common phrasal verbs used in everyday English',
   'gs://learning-app-media/uploads/2025/phrasal_verbs.mp4',
   'ready', 'ready', 'ready',
   41943040, '1280x720', 3500,
   900000000, '1280x720', 2500,
   'gs://learning-app-media/thumbnails/phrasal_verbs_thumb.jpg',
   'gs://learning-app-media/hls/phrasal_verbs/master.m3u8',
   'Intermediate', 'Learn 50+ essential phrasal verbs with practical examples',
   ARRAY['Phrasal Verbs', 'Vocabulary', 'Intermediate'],
   'gs://learning-app-media/subtitles/phrasal_verbs_en.vtt');

-- 4) published 状态（已发布上线）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate,
  duration_micros, encoded_resolution, encoded_bitrate,
  thumbnail_url, hls_master_playlist,
  difficulty, summary, tags, raw_subtitle_url
) VALUES
  (:'test_user_id', 'English Listening: News Report',
   'Practice listening comprehension with real news reports',
   'gs://learning-app-media/uploads/2025/news_listening.mp4',
   'published', 'ready', 'ready',
   62914560, '1920x1080', 4800,
   720000000, '1920x1080', 3500,
   'gs://learning-app-media/thumbnails/news_listening_thumb.jpg',
   'gs://learning-app-media/hls/news_listening/master.m3u8',
   'Upper-Intermediate', 'Improve listening skills through authentic news content',
   ARRAY['Listening', 'News', 'Upper-Intermediate', 'Current Events'],
   'gs://learning-app-media/subtitles/news_listening_en.vtt'),

  (:'test_user_id', 'American vs British English',
   'Key differences between American and British English',
   'gs://learning-app-media/uploads/2025/american_vs_british.mp4',
   'published', 'ready', 'ready',
   83886080, '1920x1080', 5200,
   1500000000, '1920x1080', 4000,
   'gs://learning-app-media/thumbnails/american_british_thumb.jpg',
   'gs://learning-app-media/hls/american_british/master.m3u8',
   'Intermediate', 'Explore vocabulary, pronunciation and spelling differences',
   ARRAY['Pronunciation', 'Vocabulary', 'Cultural', 'Intermediate'],
   'gs://learning-app-media/subtitles/american_british_en.vtt'),

  (:'test_user_id', 'Basic English for Travelers',
   'Essential English phrases for traveling abroad',
   'gs://learning-app-media/uploads/2025/travel_english.mp4',
   'published', 'ready', 'ready',
   52428800, '1280x720', 4000,
   1080000000, '1280x720', 3000,
   'gs://learning-app-media/thumbnails/travel_english_thumb.jpg',
   'gs://learning-app-media/hls/travel_english/master.m3u8',
   'Beginner', 'Learn practical English for common travel situations',
   ARRAY['Travel', 'Beginner', 'Conversation', 'Practical'],
   'gs://learning-app-media/subtitles/travel_english_en.vtt'),

  (:'test_user_id', 'Academic Writing: Essay Structure',
   'Master the fundamentals of academic essay writing',
   'gs://learning-app-media/uploads/2025/academic_writing.mp4',
   'published', 'ready', 'ready',
   104857600, '1920x1080', 6500,
   1800000000, '1920x1080', 5000,
   'gs://learning-app-media/thumbnails/academic_writing_thumb.jpg',
   'gs://learning-app-media/hls/academic_writing/master.m3u8',
   'Advanced', 'Comprehensive guide to structuring academic essays with examples',
   ARRAY['Writing', 'Academic', 'Advanced', 'Essay'],
   'gs://learning-app-media/subtitles/academic_writing_en.vtt');

-- 5) failed 状态（处理失败的视频）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate,
  error_message
) VALUES
  (:'test_user_id', 'Corrupted Upload Test',
   'This video file was corrupted during upload',
   'gs://learning-app-media/uploads/2025/corrupted_file.mp4',
   'failed', 'failed', 'pending',
   31457280, '1920x1080', 4500,
   'FFmpeg error: Invalid data found when processing input'),

  (:'test_user_id', 'AI Analysis Timeout',
   'Video processed but AI analysis timed out',
   'gs://learning-app-media/uploads/2025/ai_timeout.mp4',
   'failed', 'ready', 'failed',
   67108864, '1920x1080', 5000,
   'AI analysis service timeout after 300 seconds');

-- 6) rejected 状态（审核拒绝）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate,
  duration_micros, encoded_resolution, encoded_bitrate,
  thumbnail_url, hls_master_playlist,
  error_message
) VALUES
  (:'test_user_id', 'Inappropriate Content Example',
   'This video was rejected due to policy violation',
   'gs://learning-app-media/uploads/2025/policy_violation.mp4',
   'rejected', 'ready', 'ready',
   45088768, '1280x720', 3800,
   600000000, '1280x720', 2800,
   'gs://learning-app-media/thumbnails/rejected_thumb.jpg',
   'gs://learning-app-media/hls/rejected/master.m3u8',
   'Content moderation: Contains prohibited material');

-- 7) archived 状态（已归档）
INSERT INTO catalog.videos (
  upload_user_id, title, description, raw_file_reference,
  status, media_status, analysis_status,
  raw_file_size, raw_resolution, raw_bitrate,
  duration_micros, encoded_resolution, encoded_bitrate,
  thumbnail_url, hls_master_playlist,
  difficulty, summary, tags, raw_subtitle_url
) VALUES
  (:'test_user_id', '[ARCHIVED] Old English Course 2020',
   'Archived course content from 2020 - replaced by newer version',
   'gs://learning-app-media/uploads/2020/old_course.mp4',
   'archived', 'ready', 'ready',
   125829120, '1280x720', 4200,
   2400000000, '1280x720', 3200,
   'gs://learning-app-media/thumbnails/old_course_thumb.jpg',
   'gs://learning-app-media/hls/old_course/master.m3u8',
   'Intermediate', 'Legacy course content - archived for reference',
   ARRAY['Archived', 'Legacy', 'Reference'],
   'gs://learning-app-media/subtitles/old_course_en.vtt');

-- 验证插入结果
SELECT
  status,
  COUNT(*) as count,
  ARRAY_AGG(title ORDER BY created_at DESC) as sample_titles
FROM catalog.videos
WHERE upload_user_id = :'test_user_id'
GROUP BY status
ORDER BY status;

-- 总数统计
SELECT COUNT(*) as total_videos FROM catalog.videos WHERE upload_user_id = :'test_user_id';
