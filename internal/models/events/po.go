package events

import (
	"fmt"
	"time"

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
)

func (k Kind) String() string {
	switch k {
	case KindVideoCreated:
		return "video.created"
	case KindVideoUpdated:
		return "video.updated"
	case KindVideoDeleted:
		return "video.deleted"
	default:
		return "video.unknown"
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
	PublishedAt       *time.Time
}

// VideoDeleted 描述视频删除事件的业务载荷。
type VideoDeleted struct {
	VideoID   uuid.UUID
	DeletedAt *time.Time
	Reason    *string
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
)
