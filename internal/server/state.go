package server

import (
	"log/slog"
	"net/http"

	"localslack/internal/service"
)

// stateConversations dumps every conversation with members and messages.
// Unauthenticated debug endpoint: LocalSlack is a local-only tool.
func (h handlers) stateConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bot, err := h.svc.Authenticate(ctx, "xoxb-state")
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	states, err := h.svc.Snapshot(ctx, bot)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(states))
	for _, c := range states {
		out = append(out, stateConversation(c))
	}
	writeJSON(ctx, h.log, w, http.StatusOK, map[string]any{"ok": true, "conversations": out})
}

func stateConversation(c service.ConversationState) map[string]any {
	msgs := make([]map[string]any, 0, len(c.Messages))
	for _, m := range c.Messages {
		msgs = append(msgs, map[string]any{
			"ts": m.TS, "user": m.UserID, "text": m.Text, "thread_ts": m.ThreadTS, "reply_count": m.ReplyCount,
		})
	}
	return map[string]any{
		"id": c.ID, "name": c.Name, "kind": c.Kind, "is_archived": c.IsArchived,
		"members": c.Members, "messages": msgs,
	}
}

func (h handlers) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.ErrorContext(r.Context(), "state endpoint", slog.Any("error", err))
	writeJSON(r.Context(), h.log, w, http.StatusInternalServerError, errorBody("internal_error"))
}
