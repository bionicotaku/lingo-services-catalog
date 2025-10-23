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
	MediaStatus       StageStatus // 媒体阶段状态
	AnalysisStatus    StageStatus // AI 阶段状态
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
	RawSubtitleURL    *string     // 原始字幕/ASR 输出
	ErrorMessage      *string     // 最近一次失败/拒绝原因
}
