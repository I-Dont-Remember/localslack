package api

import (
	"context"
	"encoding/json"
	"net/url"

	"localslack/internal/service"
)

// writeContent reads text/blocks/attachments/metadata params; non-JSON values are rejected.
func writeContent(p url.Values) (service.Content, error) {
	c := service.Content{Text: p.Get("text")}
	for _, f := range []struct {
		key string
		dst *json.RawMessage
		err error
	}{
		{"blocks", &c.Blocks, service.ErrInvalidBlocks},
		{"attachments", &c.Attachments, service.ErrInvalidAttachments},
		{"metadata", &c.Metadata, service.ErrInvalidMetadataFormat},
	} {
		raw := p.Get(f.key)
		if raw == "" {
			continue
		}
		if !json.Valid([]byte(raw)) {
			return service.Content{}, f.err
		}
		*f.dst = json.RawMessage(raw)
	}
	return c, nil
}

func (a *API) chatUpdate(ctx context.Context, c *call) (map[string]any, error) {
	content, err := writeContent(c.params)
	if err != nil {
		return nil, err
	}
	channel := c.params.Get("channel")
	msg, err := a.svc.UpdateMessage(ctx, c.p.User, service.UpdateOpts{Channel: channel, TS: c.params.Get("ts"), Content: content})
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "ts": msg.TS, "text": msg.Text, "message": messageDTO(msg)}, nil
}

func (a *API) writeMethods() map[string]spec {
	return map[string]spec{
		"chat.update": tok(a.chatUpdate),
	}
}
