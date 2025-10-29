package outboxevents

import (
	"fmt"
	"time"

	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/google/uuid"
)

// Kind 标识领域事件类型。
type Kind int

// 领域事件类型常量。
const (
	// KindUnknown 表示未识别的事件类型。
	KindUnknown Kind = iota
	// KindVideoCreated 表示视频创建事件。
	KindVideoCreated
	// KindVideoUpdated 表示视频更新事件。
	KindVideoUpdated
	// KindVideoDeleted 表示视频删除事件。
	KindVideoDeleted
	// KindVideoMediaReady 表示媒体阶段完成事件。
	KindVideoMediaReady
	// KindVideoAIEnriched 表示 AI 阶段完成事件。
	KindVideoAIEnriched
	// KindVideoProcessingFailed 表示处理阶段失败事件。
	KindVideoProcessingFailed
	// KindVideoVisibilityChanged 表示可见性变更事件。
	KindVideoVisibilityChanged
)

func (k Kind) String() string {
	switch k {
	case KindVideoCreated:
		return "catalog.video.created"
	case KindVideoUpdated:
		return "catalog.video.updated"
	case KindVideoDeleted:
		return "catalog.video.deleted"
	case KindVideoMediaReady:
		return "catalog.video.media_ready"
	case KindVideoAIEnriched:
		return "catalog.video.ai_enriched"
	case KindVideoProcessingFailed:
		return "catalog.video.processing_failed"
	case KindVideoVisibilityChanged:
		return "catalog.video.visibility_changed"
	default:
		return "catalog.video.unknown"
	}
}

// DomainEvent 表示领域层生成的标准事件。
type DomainEvent struct {
	EventID       uuid.UUID
	Kind          Kind
	AggregateID   uuid.UUID
	AggregateType string
	Version       int64
	OccurredAt    time.Time
	Payload       any
}

// VideoCreated 描述视频创建事件的业务载荷。
type VideoCreated struct {
	VideoID        uuid.UUID
	UploaderID     uuid.UUID
	Title          string
	Description    *string
	DurationMicros *int64
	PublishedAt    *time.Time
	Status         string
	MediaStatus    string
	AnalysisStatus string
}

// VideoUpdated 描述视频更新事件的业务载荷。
type VideoUpdated struct {
	VideoID           uuid.UUID
	Title             *string
	Description       *string
	Status            *string
	MediaStatus       *string
	AnalysisStatus    *string
	DurationMicros    *int64
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	Difficulty        *string
	Summary           *string
	Tags              []string
	RawSubtitleURL    *string
	VisibilityStatus  *string
	PublishedAt       *time.Time
}

// VideoDeleted 描述视频删除事件的业务载荷。
type VideoDeleted struct {
	VideoID   uuid.UUID
	DeletedAt *time.Time
	Reason    *string
}

// VideoMediaReady 描述媒体阶段完成事件载荷。
type VideoMediaReady struct {
	VideoID           uuid.UUID
	Status            po.VideoStatus
	MediaStatus       po.StageStatus
	AnalysisStatus    po.StageStatus
	DurationMicros    *int64
	EncodedResolution *string
	EncodedBitrate    *int32
	ThumbnailURL      *string
	HLSMasterPlaylist *string
	JobID             *string
	EmittedAt         *time.Time
}

// VideoAIEnriched 描述 AI 阶段完成事件载荷。
type VideoAIEnriched struct {
	VideoID        uuid.UUID
	Status         po.VideoStatus
	AnalysisStatus po.StageStatus
	MediaStatus    po.StageStatus
	Difficulty     *string
	Summary        *string
	Tags           []string
	RawSubtitleURL *string
	JobID          *string
	EmittedAt      *time.Time
	ErrorMessage   *string
}

// VideoProcessingFailed 描述处理阶段失败事件载荷。
type VideoProcessingFailed struct {
	VideoID        uuid.UUID
	Status         po.VideoStatus
	MediaStatus    po.StageStatus
	AnalysisStatus po.StageStatus
	Stage          string
	ErrorMessage   *string
	JobID          *string
	EmittedAt      *time.Time
}

// VideoVisibilityChanged 描述可见性变更事件载荷。
type VideoVisibilityChanged struct {
	VideoID          uuid.UUID
	Status           po.VideoStatus
	VisibilityStatus string
	PreviousStatus   *po.VideoStatus
	PublishedAt      *time.Time
	Reason           *string
}

const (
	// AggregateTypeVideo 标识视频聚合类型，供 Outbox headers / attributes 使用。
	AggregateTypeVideo = "video"
	// SchemaVersionV1 描述事件载荷的当前 schema 版本。
	SchemaVersionV1 = "v1"
)

var (
	// ErrNilVideo 在构建事件时视频实体为空。
	ErrNilVideo = fmt.Errorf("event builder: video is nil")
	// ErrInvalidEventID 表示未提供合法的事件 ID。
	ErrInvalidEventID = fmt.Errorf("event builder: event id is required")
	// ErrUnknownEventKind 表示未识别的事件类型。
	ErrUnknownEventKind = fmt.Errorf("event builder: unknown event kind")
	// ErrInvalidStage 表示阶段名称非法。
	ErrInvalidStage = fmt.Errorf("event builder: invalid stage")
)
