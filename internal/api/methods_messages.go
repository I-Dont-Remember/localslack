package api

import (
	"context"

	"localslack/internal/service"
)

func (a *API) chatPostMessage(ctx context.Context, c *call) (map[string]any, error) {
	channel := c.params.Get("channel")
	msg, err := a.svc.PostMessage(ctx, c.p.User, service.PostOpts{
		Channel: channel, ThreadTS: c.params.Get("thread_ts"), Content: contentParams(c),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "ts": msg.TS, "message": messageDTO(msg)}, nil
}

func (a *API) conversationsArchive(ctx context.Context, c *call) (map[string]any, error) {
	if err := a.svc.ArchiveChannel(ctx, c.p.User, c.params.Get("channel")); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (a *API) messageMethods() map[string]spec {
	return map[string]spec{"chat.postMessage": tok(a.chatPostMessage)}
}
