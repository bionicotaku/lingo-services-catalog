// Package po 定义面向持久化的数据对象（Persistent Objects），由 Repository 层使用。
// PO 对象映射数据库表结构，不直接暴露给上层业务逻辑。
//
// 注意：本包仅保留枚举类型定义，供 sqlc 生成代码引用。
// 实体模型由 sqlc 自动生成（见 internal/repositories/sqlc/models.go）。
package po

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
