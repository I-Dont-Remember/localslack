package handlers

import (
	"context"
	"localslack/internal/core"
	"localslack/internal/utils"
)

func HandleUsersList(ctx context.Context, req utils.ApiRequest, d utils.Deps) (any, *utils.SlackError) {
	// TODO: fake this out for now

	members := []core.SlackUser{
		{
			ID:   "FAKE_USER_ID",
			Name: "FAKE_USER_NAME",
			Profile: core.SlackUserProfile{
				Team:     "FAKE_TEAM_ID",
				RealName: "John Doe",
			},
		},
	}
	return map[string]any{
		"ok":       true,
		"members":  members,
		"cache_ts": 0,
		"response_metadata": map[string]string{
			"next_cursor": "dXNlcjpVMEc5V0ZYTlo=",
		},
	}, nil
}
