// Package uploads implements the OBJECT_FINALIZE ingestion pipeline.
package uploads

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Event 表示从 GCS OBJECT_FINALIZE 消息中解析出的关键信息。
type Event struct {
	Bucket      string
	ObjectName  string
	Generation  string
	SizeBytes   int64
	ContentType string
	MD5Base64   string
	CRC32C      string
	ETag        string
}

type gcsObjectMessage struct {
	Bucket      string `json:"bucket"`
	Name        string `json:"name"`
	Generation  string `json:"generation"`
	Size        string `json:"size"`
	ContentType string `json:"contentType"`
	MD5Hash     string `json:"md5Hash"`
	CRC32C      string `json:"crc32c"`
	ETag        string `json:"etag"`
}

type eventDecoder struct{}

func newDecoder() *eventDecoder {
	return &eventDecoder{}
}

// Decode 将 Pub/Sub 消息数据解析为 Event。
func (d *eventDecoder) Decode(data []byte) (*Event, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("uploads: empty payload")
	}

	var msg gcsObjectMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("uploads: decode gcs object payload: %w", err)
	}

	if msg.Bucket == "" || msg.Name == "" {
		return nil, fmt.Errorf("uploads: missing bucket or object name")
	}

	var size int64
	if msg.Size != "" {
		parsed, err := strconv.ParseInt(msg.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("uploads: parse size: %w", err)
		}
		size = parsed
	}

	return &Event{
		Bucket:      msg.Bucket,
		ObjectName:  msg.Name,
		Generation:  msg.Generation,
		SizeBytes:   size,
		ContentType: msg.ContentType,
		MD5Base64:   msg.MD5Hash,
		CRC32C:      msg.CRC32C,
		ETag:        msg.ETag,
	}, nil
}
