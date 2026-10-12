package api

import (
	"context"

	"localslack/internal/service"
)

func (a *API) readMethods() map[string]spec {
	return map[string]spec{
		"api.test":              {fn: a.apiTest, auth: authNone},
		"conversations.info":    tok(a.conversationsInfo),
		"conversations.members": tok(a.conversationsMembers),
		"users.info":            tok(a.usersInfo),
	}
}

func (a *API) conversationsInfo(ctx context.Context, c *call) (map[string]any, error) {
	conv, err := a.svc.Conversation(ctx, c.p.User, c.params.Get("channel"))
	if err != nil {
		return nil, err
	}
	dto := conversationDTO(conv)
	if !boolParam(c.params, "include_num_members") {
		delete(dto, "num_members")
	}
	return map[string]any{"channel": dto}, nil
}

func (a *API) conversationsMembers(ctx context.Context, c *call) (map[string]any, error) {
	page, err := a.svc.Members(ctx, c.p.User, c.params.Get("channel"), service.PageOpts{
		Cursor: c.params.Get("cursor"), Limit: intParam(c.params, "limit"),
	})
	if err != nil {
		return nil, err
	}
	members := page.Items
	if members == nil {
		members = []string{}
	}
	return map[string]any{"members": members, "response_metadata": cursorMeta(page.NextCursor)}, nil
}

func (a *API) usersInfo(ctx context.Context, c *call) (map[string]any, error) {
	team, err := a.svc.Team(ctx)
	if err != nil {
		return nil, err
	}
	u, err := a.svc.User(ctx, c.params.Get("user"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": userDTO(u, team.ID)}, nil
}

// apiTest echoes its params (never the token); `error=x` makes it fail with x.
func (a *API) apiTest(_ context.Context, c *call) (map[string]any, error) {
	args := map[string]any{}
	for k := range c.params {
		if k != "token" {
			args[k] = c.params.Get(k)
		}
	}
	out := map[string]any{"ok": true}
	if len(args) > 0 {
		out["args"] = args
	}
	if e := c.params.Get("error"); e != "" {
		out["ok"], out["error"] = false, e
	}
	return out, nil
}
