package service_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"localslack/internal/service"
)

func dividers(n int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat(`{"type":"divider"},`, n), ",") + "]")
}

func TestValidateContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		c    service.Content
		want error
	}{
		{"text only", service.Content{Text: "hi"}, nil},
		{"nothing", service.Content{}, service.ErrNoText},
		{"empty arrays only", service.Content{Blocks: json.RawMessage(`[]`), Attachments: json.RawMessage(`[]`)}, service.ErrNoText},
		{"null blocks", service.Content{Blocks: json.RawMessage(`null`)}, service.ErrNoText},
		{"blocks only", service.Content{Blocks: json.RawMessage(`[{"type":"divider"}]`)}, nil},
		{"attachments only", service.Content{Attachments: json.RawMessage(`[{"text":"a"}]`)}, nil},
		{"blocks not json", service.Content{Text: "x", Blocks: json.RawMessage(`nope`)}, service.ErrInvalidBlocks},
		{"blocks object", service.Content{Text: "x", Blocks: json.RawMessage(`{"type":"divider"}`)}, service.ErrInvalidBlocks},
		{"block not object", service.Content{Text: "x", Blocks: json.RawMessage(`["divider"]`)}, service.ErrInvalidBlocks},
		{"block no type", service.Content{Text: "x", Blocks: json.RawMessage(`[{"text":"a"}]`)}, service.ErrInvalidBlocks},
		{"block type not string", service.Content{Text: "x", Blocks: json.RawMessage(`[{"type":1}]`)}, service.ErrInvalidBlocks},
		{"50 blocks", service.Content{Blocks: dividers(50)}, nil},
		{"51 blocks", service.Content{Blocks: dividers(51)}, service.ErrInvalidBlocks},
		{"attachments not array", service.Content{Text: "x", Attachments: json.RawMessage(`{"a":1}`)}, service.ErrInvalidAttachments},
		{"attachment not object", service.Content{Text: "x", Attachments: json.RawMessage(`[1]`)}, service.ErrInvalidAttachments},
		{"metadata ok", service.Content{Text: "x", Metadata: json.RawMessage(`{"event_type":"t","event_payload":{"k":1}}`)}, nil},
		{"metadata not object", service.Content{Text: "x", Metadata: json.RawMessage(`[]`)}, service.ErrInvalidMetadataFormat},
		{"metadata no type", service.Content{Text: "x", Metadata: json.RawMessage(`{"event_payload":{}}`)}, service.ErrInvalidMetadataFormat},
		{"metadata empty type", service.Content{Text: "x", Metadata: json.RawMessage(`{"event_type":"","event_payload":{}}`)}, service.ErrInvalidMetadataFormat},
		{"metadata no payload", service.Content{Text: "x", Metadata: json.RawMessage(`{"event_type":"t"}`)}, service.ErrInvalidMetadataFormat},
		{"metadata payload not object", service.Content{Text: "x", Metadata: json.RawMessage(`{"event_type":"t","event_payload":"s"}`)}, service.ErrInvalidMetadataFormat},
		{"text at limit", service.Content{Text: strings.Repeat("é", 40000)}, nil},
		{"text over limit", service.Content{Text: strings.Repeat("é", 40001)}, service.ErrMsgTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := service.ValidateContent(tt.c); !errors.Is(got, tt.want) {
				t.Errorf("ValidateContent() = %v, want %v", got, tt.want)
			}
		})
	}
}
