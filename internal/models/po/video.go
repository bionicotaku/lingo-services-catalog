// Package po 定义面向持久化的数据对象（Persistent Objects），由 Repository 层使用。
// PO 对象映射数据库表结构，不直接暴露给上层业务逻辑。
//
// 注意：枚举类型供 sqlc 生成代码引用；Video 结构体作为仓储返回的统一实体，
// 便于 Service 与 VO 层解耦底层 sqlc 模型。
package po

import (
	"time"

	"github.com/google/uuid"
)

// VideoStatus 表示视频的总体生命周期状态。
// 对应数据库枚举类型 catalog.video_status。
type VideoStatus string

// 视频状态常量定义
const (
	VideoStatusPendingUpload VideoStatus = "pending_upload" // 记录已创建但上传未完成
	VideoStatusProcessing    VideoStatus = "processing"     // 媒体或分析阶段仍在进行
	VideoStatusReady         VideoStatus = "ready"          // 媒体与分析阶段均完成
	VideoStatusPublished     VideoStatus = "published"      // 已上架对外可见
	VideoStatusFailed        VideoStatus = "failed"         // 任一阶段失败
	VideoStatusRejected      VideoStatus = "rejected"       // 审核拒绝或强制下架
	VideoStatusArchived      VideoStatus = "archived"       // 主动归档或长期下架
)

// VideoMetadata 表示主表中的媒体/AI 元数据快照。
type VideoMetadata struct {
	VideoID           uuid.UUID
	Status            VideoStatus
	MediaStatus       StageStatus
	AnalysisStatus    StageStatus
	DurationMicros    *int64
	EncodedResolution *string
	EncodedBitrate    *int32
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	Tags              []string
	RawSubtitleURL    *string
	VisibilityStatus  string
	PublishAt         *time.Time
	UpdatedAt         time.Time
	Version           int64
}

// StageStatus 表示分阶段执行状态。
// 对应数据库枚举类型 catalog.stage_status。
type StageStatus string

// 阶段状态常量定义
const (
	StagePending    StageStatus = "pending"    // 尚未开始该阶段
	StageProcessing StageStatus = "processing" // 阶段执行中
	StageReady      StageStatus = "ready"      // 阶段完成
	StageFailed     StageStatus = "failed"     // 阶段失败
)

// Visibility 状态取值。
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
)

// Video 表示 catalog.videos 表的数据库实体。
// 仓储层将 sqlc 生成的模型转换为该结构体，避免外层依赖具体 ORM。
type Video struct {
	VideoID           uuid.UUID   // 主键
	UploadUserID      uuid.UUID   // 上传者
	CreatedAt         time.Time   // 创建时间
	UpdatedAt         time.Time   // 最近更新时间
	Title             string      // 标题
	Description       *string     // 视频描述
	RawFileReference  string      // 原始对象引用
	Status            VideoStatus // 总体状态
	Version           int64       // 乐观锁版本号
	MediaStatus       StageStatus // 媒体阶段状态
	AnalysisStatus    StageStatus // AI 阶段状态
	MediaJobID        *string     // 最近一次媒体任务 ID
	MediaEmittedAt    *time.Time  // 最近一次媒体任务完成时间
	AnalysisJobID     *string     // 最近一次 AI 任务 ID
	AnalysisEmittedAt *time.Time  // 最近一次 AI 任务完成时间
	RawFileSize       *int64      // 原始文件大小（字节）
	RawResolution     *string     // 原始分辨率
	RawBitrate        *int32      // 原始码率（kbps）
	DurationMicros    *int64      // 视频时长（微秒）
	EncodedResolution *string     // 转码后分辨率
	EncodedBitrate    *int32      // 转码后码率（kbps）
	ThumbnailURL      *string     // 主缩略图 URL
	HLSMasterPlaylist *string     // HLS master playlist URL
	Difficulty        *string     // AI 评估难度
	Summary           *string     // AI 生成摘要
	Tags              []string    // AI 生成标签
	VisibilityStatus  string      // 可见性状态（public/unlisted/private）
	PublishAt         *time.Time  // 发布时间（UTC）
	RawSubtitleURL    *string     // 原始字幕/ASR 输出
	ErrorMessage      *string     // 最近一次失败/拒绝原因
}

// VideoReadyView 表示从 catalog.videos 主表读取的只读视图。
// 仅携带展示层所需字段，限制状态在 ready/published 范围。
type VideoReadyView struct {
	VideoID          uuid.UUID   // 主键
	Title            string      // 标题
	Status           VideoStatus // 总体状态
	MediaStatus      StageStatus // 媒体阶段状态
	AnalysisStatus   StageStatus // AI 阶段状态
	CreatedAt        time.Time   // 创建时间
	UpdatedAt        time.Time   // 最近更新时间
	VisibilityStatus string      // 当前可见性状态
	PublishAt        *time.Time
}

// VideoUserState 表示用户与视频的互动状态投影。
// 数据来源：catalog.video_user_engagements_projection 表，由 Engagement 投影消费者维护。
type VideoUserState struct {
	UserID               uuid.UUID  // 用户主键
	VideoID              uuid.UUID  // 视频主键
	HasLiked             bool       // 是否点赞
	HasBookmarked        bool       // 是否收藏
	LikedOccurredAt      *time.Time // 最近一次点赞事件时间
	BookmarkedOccurredAt *time.Time // 最近一次收藏事件时间
	UpdatedAt            time.Time  // 最后一次更新的时间
}

// VideoListEntry 表示来自主表的视频条目。
type VideoListEntry struct {
	VideoID          uuid.UUID
	Title            string
	Status           VideoStatus
	MediaStatus      StageStatus
	AnalysisStatus   StageStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	VisibilityStatus string
	PublishAt        *time.Time
}

// MyUploadEntry 表示用户上传的视频条目。
type MyUploadEntry struct {
	VideoID          uuid.UUID
	Title            string
	Status           VideoStatus
	MediaStatus      StageStatus
	AnalysisStatus   StageStatus
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	VisibilityStatus string
	PublishAt        *time.Time
}
