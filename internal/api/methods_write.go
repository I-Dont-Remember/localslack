package api

import (
	"context"

	"localslack/internal/service"
)

// writeContent reads and validates message content for update/ephemeral calls.
func writeContent(c *call) (service.Content, error) {
	content := contentParams(c)
	return content, service.ValidateContent(content)
}

func (a *API) chatUpdate(ctx context.Context, c *call) (map[string]any, error) {
	content, err := writeContent(c)
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

func (a *API) chatDelete(ctx context.Context, c *call) (map[string]any, error) {
	channel, ts := c.params.Get("channel"), c.params.Get("ts")
	if err := a.svc.DeleteMessage(ctx, c.p.User, channel, ts); err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "ts": ts}, nil
}

func (a *API) chatPostEphemeral(ctx context.Context, c *call) (map[string]any, error) {
	content, err := writeContent(c)
	if err != nil {
		return nil, err
	}
	ts, err := a.svc.PostEphemeral(ctx, c.p.User, service.EphemeralOpts{
		Channel: c.params.Get("channel"), User: c.params.Get("user"), ThreadTS: c.params.Get("thread_ts"), Content: content,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"message_ts": ts}, nil
}

func (a *API) writeMethods() map[string]spec {
	return map[string]spec{
		"chat.postEphemeral": tok(a.chatPostEphemeral),
		"chat.update":        tok(a.chatUpdate),
		"chat.delete":        tok(a.chatDelete),
	}
}
