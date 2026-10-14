package api

import "localslack/internal/service"

// addReactions adds reactions[] when the message has any.
func addReactions(out map[string]any, m service.Message) {
	if len(m.Reactions) == 0 {
		return
	}
	rs := make([]map[string]any, 0, len(m.Reactions))
	for _, r := range m.Reactions {
		rs = append(rs, map[string]any{"name": r.Name, "users": r.Users, "count": r.Count})
	}
	out["reactions"] = rs
}
