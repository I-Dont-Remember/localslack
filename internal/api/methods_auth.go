package api

import "context"

func (a *API) authTest(ctx context.Context, c *call) (map[string]any, error) {
	team, err := a.svc.Team(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"url": c.baseURL, "team": team.Name, "user": c.user.Name,
		"team_id": team.ID, "user_id": c.user.ID, "is_enterprise_install": false,
	}
	if c.user.IsBot {
		out["bot_id"] = "B" + c.user.ID[1:]
	}
	return out, nil
}

func (a *API) usersList(ctx context.Context, _ *call) (map[string]any, error) {
	team, err := a.svc.Team(ctx)
	if err != nil {
		return nil, err
	}
	users, err := a.svc.Users(ctx)
	if err != nil {
		return nil, err
	}
	members := make([]map[string]any, 0, len(users))
	for _, u := range users {
		members = append(members, userDTO(u, team.ID))
	}
	return map[string]any{"members": members, "response_metadata": map[string]any{"next_cursor": ""}}, nil
}
