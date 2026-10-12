package api

import "context"

func (a *API) userMethods() map[string]spec {
	return map[string]spec{"users.list": tok(a.usersList)}
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
