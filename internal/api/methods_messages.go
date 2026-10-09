package api

import "context"

func (a *API) chatPostMessage(ctx context.Context, c *call) (map[string]any, error) {
	channel := c.params.Get("channel")
	msg, err := a.svc.PostMessage(ctx, c.user, channel, c.params.Get("text"), c.params.Get("thread_ts"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "ts": msg.TS, "message": messageDTO(msg)}, nil
}

func (a *API) conversationsArchive(ctx context.Context, c *call) (map[string]any, error) {
	if err := a.svc.ArchiveChannel(ctx, c.user, c.params.Get("channel")); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}
