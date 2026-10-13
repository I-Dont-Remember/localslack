package api

import (
	"context"
	"errors"

	"localslack/internal/service"
)

func (a *API) conversationsJoin(ctx context.Context, c *call) (map[string]any, error) {
	conv, already, err := a.svc.JoinChannel(ctx, c.p.User, c.params.Get("channel"))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"channel": conversationDTO(conv)}
	if already {
		out["warning"] = "already_in_channel"
		out["response_metadata"] = map[string]any{"warnings": []string{"already_in_channel"}}
	}
	return out, nil
}

func (a *API) conversationsLeave(ctx context.Context, c *call) (map[string]any, error) {
	if err := a.svc.LeaveChannel(ctx, c.p.User, c.params.Get("channel")); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (a *API) conversationsInvite(ctx context.Context, c *call) (map[string]any, error) {
	conv, err := a.svc.InviteToChannel(ctx, c.p.User, c.params.Get("channel"), csv(c.params.Get("users")), boolParam(c.params, "force"))
	var ie *service.InviteError
	if errors.As(err, &ie) {
		return inviteFailure(ie), nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"channel": conversationDTO(conv)}, nil
}

// inviteFailure is Slack's shape: the first error as `error`, every user's in `errors`.
func inviteFailure(ie *service.InviteError) map[string]any {
	errs := make([]map[string]any, 0, len(ie.Failures))
	first := ""
	for _, f := range ie.Failures {
		code, _ := ErrorCode(f.Err)
		if first == "" {
			first = code
		}
		errs = append(errs, map[string]any{"user": f.User, "ok": false, "error": code})
	}
	return map[string]any{"ok": false, "error": first, "errors": errs}
}

func (a *API) conversationsKick(ctx context.Context, c *call) (map[string]any, error) {
	if err := a.svc.KickFromChannel(ctx, c.p.User, c.params.Get("channel"), c.params.Get("user")); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (a *API) membershipMethods() map[string]spec {
	return map[string]spec{
		"conversations.join":   tok(a.conversationsJoin),
		"conversations.leave":  tok(a.conversationsLeave),
		"conversations.invite": tok(a.conversationsInvite),
		"conversations.kick":   tok(a.conversationsKick),
	}
}
