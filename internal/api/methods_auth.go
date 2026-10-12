package api

import "context"

func (a *API) authTest(ctx context.Context, c *call) (map[string]any, error) {
	team, err := a.svc.Team(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"url": c.baseURL, "team": team.Name, "user": c.p.User.Name,
		"team_id": team.ID, "user_id": c.p.User.ID, "is_enterprise_install": false,
	}
	if c.p.User.BotID != "" {
		out["bot_id"] = c.p.User.BotID
	}
	return out, nil
}

func (a *API) authMethods() map[string]spec {
	return map[string]spec{"auth.test": tok(a.authTest)}
}
