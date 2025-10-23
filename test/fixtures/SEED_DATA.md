# 测试数据说明文档

## 概览

已为测试用户 `test@test.com` (ID: `f0ad5a16-0d50-4f94-8ff7-b99dda13ee47`) 添加了 **14 个测试视频**，覆盖视频生命周期的所有状态。

## 数据分布

### 按状态分类

| 状态 | 数量 | 说明 |
|------|------|------|
| `pending_upload` | 2 | 记录已创建但上传未完成 |
| `processing` | 2 | 正在进行媒体转码或 AI 分析 |
| `ready` | 2 | 处理完成，待发布 |
| `published` | 4 | 已上架对外可见（主要测试数据） |
| `failed` | 2 | 处理失败（包含错误信息） |
| `rejected` | 1 | 审核拒绝/强制下架 |
| `archived` | 1 | 已归档的历史内容 |

### 按难度分类

| 难度 | 数量 | 视频示例 |
|------|------|----------|
| `Beginner` | 1 | Basic English for Travelers |
| `Intermediate` | 3 | Phrasal Verbs, American vs British, Old Course (archived) |
| `Upper-Intermediate` | 1 | English Listening: News Report |
| `Advanced` | 2 | IELTS Speaking, Academic Writing |
| NULL (未分析) | 7 | 待处理或失败的视频 |

## 测试场景覆盖

### 1. 正常流程视频（published，4个）

#### 1.1 初级难度
- **Basic English for Travelers**
  - 难度：Beginner
  - 时长：18 分钟 (1080s)
  - 标签：Travel, Beginner, Conversation, Practical
  - 场景：旅游实用英语

#### 1.2 中级难度
- **American vs British English**
  - 难度：Intermediate
  - 时长：25 分钟 (1500s)
  - 标签：Pronunciation, Vocabulary, Cultural, Intermediate
  - 场景：文化差异对比

#### 1.3 中高级难度
- **English Listening: News Report**
  - 难度：Upper-Intermediate
  - 时长：12 分钟 (720s)
  - 标签：Listening, News, Upper-Intermediate, Current Events
  - 场景：新闻听力练习

#### 1.4 高级难度
- **Academic Writing: Essay Structure**
  - 难度：Advanced
  - 时长：30 分钟 (1800s)
  - 文件最大：~100MB
  - 标签：Writing, Academic, Advanced, Essay
  - 场景：学术写作（最完整的测试数据）

### 2. 待发布视频（ready，2个）

- **IELTS Speaking Test Preparation**
  - 难度：Advanced
  - 时长：20 分钟
  - 完整的媒体和 AI 产物
  - 用于测试发布工作流

- **Phrasal Verbs for Daily Use**
  - 难度：Intermediate
  - 时长：15 分钟
  - 用于测试批量发布

### 3. 处理中视频（processing，2个）

- **English Grammar: Present Tense**
  - 媒体阶段：processing
  - AI 阶段：pending
  - 用于测试转码监控

- **Business English Email Writing**
  - 媒体阶段：ready
  - AI 阶段：processing
  - 用于测试 AI 分析监控

### 4. 待上传视频（pending_upload，2个）

- **Introduction to English Pronunciation**
- **Daily Conversation Practice #1**

用于测试上传工作流和状态转换。

### 5. 失败场景（failed，2个）

- **Corrupted Upload Test**
  - 媒体失败：`FFmpeg error: Invalid data found when processing input`
  - 用于测试转码错误处理

- **AI Analysis Timeout**
  - AI 失败：`AI analysis service timeout after 300 seconds`
  - 媒体已完成，但 AI 超时
  - 用于测试重试机制

### 6. 拒绝场景（rejected，1个）

- **Inappropriate Content Example**
  - 错误信息：`Content moderation: Contains prohibited material`
  - 媒体和 AI 均已完成
  - 用于测试内容审核流程

### 7. 归档场景（archived，1个）

- **[ARCHIVED] Old English Course 2020**
  - 完整的历史数据
  - 用于测试归档查询和恢复

## 数据字段完整性

### 完整字段的视频（published 状态）
✅ 所有必填字段
✅ 原始媒体属性（file_size, resolution, bitrate）
✅ 转码产物（duration, encoded_*, thumbnail, HLS playlist）
✅ AI 产物（difficulty, summary, tags, subtitle）

### 部分字段的视频（processing/ready 状态）
✅ 基础信息 + 原始媒体属性
⏳ 部分转码/AI 产物（根据 stage_status）

### 最小字段的视频（pending_upload 状态）
✅ 仅基础信息（title, description, raw_file_reference）

### 错误信息字段（failed/rejected 状态）
✅ error_message 字段有具体错误描述

## 数据质量特点

1. **真实性**：模拟真实英语学习视频场景
2. **多样性**：覆盖初级到高级所有难度级别
3. **完整性**：包含完整的元数据、媒体和 AI 产物
4. **合理性**：文件大小、分辨率、码率符合实际情况
5. **可测性**：每种状态都有对应的测试用例

## 使用建议

### 开发测试
```sql
-- 查询所有已发布视频（Feed 场景）
SELECT * FROM catalog.videos
WHERE status = 'published'
ORDER BY created_at DESC;

-- 查询特定难度的视频（筛选场景）
SELECT * FROM catalog.videos
WHERE difficulty = 'Advanced' AND status = 'published';

-- 查询包含特定标签的视频（搜索场景）
SELECT * FROM catalog.videos
WHERE tags && ARRAY['Listening']::text[];

-- 监控待处理队列
SELECT status, media_status, analysis_status, COUNT(*)
FROM catalog.videos
GROUP BY status, media_status, analysis_status;
```

### 接口测试
使用以下 video_id 测试不同场景：

- **正常查询**：`df3c43f5-6c8f-4b25-b0f2-715b228c7a2f` (Academic Writing)
- **未发布**：`c1a81119-c1f9-4806-9fcf-e0b87599c52c` (IELTS Speaking)
- **处理中**：`519794d1-23eb-4dac-9978-ed8c41e3aa77` (Grammar)
- **失败**：`52f4ece9-46dc-4dd8-be48-b3726f9e1a0b` (Corrupted)

## 重置测试数据

如需清空并重新生成测试数据：

```bash
# 清空现有数据
source configs/.env
psql "$DATABASE_URL" -c "DELETE FROM catalog.videos WHERE upload_user_id = 'f0ad5a16-0d50-4f94-8ff7-b99dda13ee47';"

# 重新执行种子脚本
psql "$DATABASE_URL" -f migrations/seed_test_videos.sql
```

## 数据文件

- **SQL 脚本**：`migrations/seed_test_videos.sql`
- **执行日期**：2025-10-23
- **记录总数**：14 条
