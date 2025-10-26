// Package projection contains projection task helpers including event decoding.
package projection

import (
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"google.golang.org/protobuf/proto"
)

type eventDecoder struct{}

func newEventDecoder() *eventDecoder {
	return &eventDecoder{}
}

func (d *eventDecoder) Decode(data []byte) (*videov1.Event, error) {
	evt := &videov1.Event{}
	if err := proto.Unmarshal(data, evt); err != nil {
		return nil, err
	}
	return evt, nil
}
