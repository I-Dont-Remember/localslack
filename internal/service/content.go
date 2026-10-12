// Content validation shared by every write path that carries message content.
// chat.postMessage calls it through PostMessage; chat.update should call
// ValidateContent on the new content before storing it.
package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

const (
	maxBlocks   = 50
	maxTextRune = 40000
)

// ValidateContent checks c the way Slack does and returns ErrMsgTooLong,
// ErrInvalidBlocks, ErrInvalidAttachments, ErrInvalidMetadataFormat or, when
// there is no text, no blocks and no attachments, ErrNoText.
func ValidateContent(c Content) error {
	if utf8.RuneCountInString(c.Text) > maxTextRune {
		return ErrMsgTooLong
	}
	nBlocks, err := countObjects(c.Blocks, maxBlocks, true)
	if err != nil {
		return ErrInvalidBlocks
	}
	nAtt, err := countObjects(c.Attachments, -1, false)
	if err != nil {
		return ErrInvalidAttachments
	}
	if present(c.Metadata) && !validMetadata(c.Metadata) {
		return ErrInvalidMetadataFormat
	}
	if c.Text == "" && nBlocks == 0 && nAtt == 0 {
		return ErrNoText
	}
	return nil
}

// present reports whether raw carries a value (not absent, empty or JSON null).
func present(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && !bytes.Equal(t, []byte("null"))
}

var errNotObjects = errors.New("not an array of valid objects")

// countObjects decodes raw as an array of objects (each with a string "type"
// when typed) and returns its length. limit < 0 means unlimited. Absent -> 0.
func countObjects(raw json.RawMessage, limit int, typed bool) (int, error) {
	if !present(raw) {
		return 0, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, err
	}
	if limit >= 0 && len(items) > limit {
		return 0, errNotObjects
	}
	for _, it := range items {
		if it == nil {
			return 0, errNotObjects
		}
		if typed && !isString(it["type"]) {
			return 0, errNotObjects
		}
	}
	return len(items), nil
}

func isString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && len(raw) > 0
}

func validMetadata(raw json.RawMessage) bool {
	var m struct {
		EventType    string                     `json:"event_type"`
		EventPayload map[string]json.RawMessage `json:"event_payload"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	return m.EventType != "" && m.EventPayload != nil
}
