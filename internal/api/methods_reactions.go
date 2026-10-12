package api

import "context"

func (a *API) reactionsAdd(ctx context.Context, c *call) (map[string]any, error) {
	err := a.svc.AddReaction(ctx, c.p.User, c.params.Get("channel"), c.params.Get("timestamp"), c.params.Get("name"))
	if err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (a *API) reactionMethods() map[string]spec {
	return map[string]spec{"reactions.add": tok(a.reactionsAdd)}
}
