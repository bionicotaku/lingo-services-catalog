package engagement_test

import (
	"testing"

	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
)

func TestDecodePayloadCopiesBuffer(t *testing.T) {
	raw := []byte{0x01, 0x02, 0x03}
	evt, err := engagement.DecodePayload(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if evt == nil {
		t.Fatalf("expected non-nil event")
	}
	if len(evt.Payload) != len(raw) {
		t.Fatalf("unexpected payload length: %d", len(evt.Payload))
	}
	for i := range raw {
		if evt.Payload[i] != raw[i] {
			t.Fatalf("payload mismatch at index %d", i)
		}
	}
	raw[0] = 0xFF
	if evt.Payload[0] == raw[0] {
		t.Fatalf("expected decoder to copy payload buffer")
	}
}

func TestDecodePayloadRejectsEmpty(t *testing.T) {
	if _, err := engagement.DecodePayload(nil); err == nil {
		t.Fatalf("expected error for nil payload")
	}
	if _, err := engagement.DecodePayload([]byte{}); err == nil {
		t.Fatalf("expected error for empty payload")
	}
}
