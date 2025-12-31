package handlers

import (
	"context"
	"localslack/internal/core"
	"localslack/internal/utils"
)

func HandleAuthCheck(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *core.SlackError) {
	return map[string]any{
		"ok":      true,
		"user_id": "FAKE_USER_ID",
		"team_id": "FAKE_TEAM_ID",
		// ... other stuff from the token's login like team id
	}, nil
}
