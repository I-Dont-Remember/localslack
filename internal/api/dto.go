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

func conversationDTO(c service.Conversation) map[string]any {
	out := map[string]any{
		"id": c.ID, "created": c.Created, "creator": c.Creator, "is_archived": c.IsArchived,
		"is_channel": false, "is_group": false, "is_im": false, "is_mpim": false, "is_private": false,
		"is_member": c.IsMember, "num_members": c.NumMembers,
	}
	switch c.Kind {
	case service.KindChannel:
		out["is_channel"] = true
		out["name"] = c.Name
		out["name_normalized"] = c.Name
		out["is_general"] = c.Name == "general"
	case service.KindIM:
		out["is_im"] = true
		out["is_private"] = true
		out["user"] = c.Peer
		out["is_user_deleted"] = false
	case service.KindMPIM:
		out["is_mpim"] = true
		out["is_group"] = true
		out["is_private"] = true
		out["name"] = c.Name
	}
	return out
}

func messageDTO(m service.Message) map[string]any {
	out := map[string]any{"type": "message", "user": m.UserID, "text": m.Text, "ts": m.TS}
	if m.ThreadTS != "" {
		out["thread_ts"] = m.ThreadTS
	}
	if m.ReplyCount > 0 {
		out["reply_count"] = m.ReplyCount
		out["latest_reply"] = m.LatestReply
	}
	if m.ParentUserID != "" {
		out["parent_user_id"] = m.ParentUserID
	}
	return out
}

func cursorMeta(next string) map[string]any { return map[string]any{"next_cursor": next} }
