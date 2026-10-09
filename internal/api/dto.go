package api

import "localslack/internal/service"

// DTO mappers build Slack-shaped JSON. Service types are never marshaled directly.

func userDTO(u service.User, teamID string) map[string]any {
	return map[string]any{
		"id": u.ID, "team_id": teamID, "name": u.Name, "real_name": u.RealName,
		"deleted": false, "is_bot": u.IsBot,
		"profile": map[string]any{"real_name": u.RealName, "display_name": u.Name},
	}
}
