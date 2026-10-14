package api

import (
	"context"

	"localslack/internal/service"
)

func (a *API) userMethods() map[string]spec {
	return map[string]spec{"users.list": tok(a.usersList)}
}

func (a *API) usersList(ctx context.Context, c *call) (map[string]any, error) {
	team, err := a.svc.Team(ctx)
	if err != nil {
		return nil, err
	}
	page, err := a.svc.UsersPage(ctx, service.PageOpts{Cursor: c.params.Get("cursor"), Limit: intParam(c.params, "limit")})
	if err != nil {
		return nil, err
	}
	members := make([]map[string]any, 0, len(page.Items))
	for _, u := range page.Items {
		members = append(members, userDTO(u, team.ID))
	}
	return map[string]any{"members": members, "response_metadata": cursorMeta(page.NextCursor)}, nil
}
