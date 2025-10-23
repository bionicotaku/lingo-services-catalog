// Package po 定义面向持久化的数据对象（Persistent Objects），由 Repository 层使用。
// PO 对象映射数据库表结构，不直接暴露给上层业务逻辑。
package po

import (
	"time"

	"github.com/google/uuid"
)

// VideoStatus 表示视频的总体生命周期状态
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

// StageStatus 表示分阶段执行状态
type StageStatus string

// 阶段状态常量定义
const (
	StagePending    StageStatus = "pending"    // 尚未开始该阶段
	StageProcessing StageStatus = "processing" // 阶段执行中
	StageReady      StageStatus = "ready"      // 阶段完成
	StageFailed     StageStatus = "failed"     // 阶段失败
)

// Video 表示 catalog.videos 表的数据库实体。
// 映射视频元数据的完整生命周期：上传 → 转码 → AI分析 → 发布。
type Video struct {
	// ============================================
	// 基础层字段
	// ============================================
	VideoID          uuid.UUID   `db:"video_id"`           // 主键（UUID v4）
	UploadUserID     uuid.UUID   `db:"upload_user_id"`     // 上传者用户 ID（外键 auth.users）
	CreatedAt        time.Time   `db:"created_at"`         // 记录创建时间
	UpdatedAt        time.Time   `db:"updated_at"`         // 最近更新时间（触发器自动维护）
	Title            string      `db:"title"`              // 视频标题（必填）
	Description      *string     `db:"description"`        // 视频描述（可选）
	RawFileReference string      `db:"raw_file_reference"` // 原始文件对象路径（GCS/S3）
	Status           VideoStatus `db:"status"`             // 总体状态
	MediaStatus      StageStatus `db:"media_status"`       // 媒体流水线阶段状态
	AnalysisStatus   StageStatus `db:"analysis_status"`    // AI 分析阶段状态

	// ============================================
	// 上传完成后补写的原始媒体属性
	// ============================================
	RawFileSize   *int64  `db:"raw_file_size"`   // 原始文件大小（字节，>0 约束）
	RawResolution *string `db:"raw_resolution"`  // 原始分辨率（如 "3840x2160"）
	RawBitrate    *int32  `db:"raw_bitrate"`     // 原始码率（kbps）

	// ============================================
	// 媒体转码完成后补写
	// ============================================
	DurationMicros     *int64  `db:"duration_micros"`      // 视频时长（微秒，高精度）
	EncodedResolution  *string `db:"encoded_resolution"`   // 主转码分辨率（如 "1920x1080"）
	EncodedBitrate     *int32  `db:"encoded_bitrate"`      // 主转码码率（kbps）
	ThumbnailURL       *string `db:"thumbnail_url"`        // 主缩略图 URL/路径
	HLSMasterPlaylist  *string `db:"hls_master_playlist"`  // HLS master.m3u8 的 URL/路径

	// ============================================
	// AI 分析完成后补写
	// ============================================
	Difficulty       *string  `db:"difficulty"`        // AI 评估难度
	Summary          *string  `db:"summary"`           // AI 生成摘要
	Tags             []string `db:"tags"`              // AI 生成标签（PostgreSQL text[]）
	RawSubtitleURL   *string  `db:"raw_subtitle_url"`  // 原始字幕/ASR 输出 URL

	// ============================================
	// 错误与审计
	// ============================================
	ErrorMessage *string `db:"error_message"` // 最近一次失败/拒绝原因
}
