package admin

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/service"
)

func mountState(r chi.Router, d Deps) {
	if d.Svc == nil {
		return
	}
	r.Get("/_localslack/state/conversations", d.stateConversations)
}

// stateConversations dumps every conversation with members and messages.
// Unauthenticated debug endpoint: LocalSlack is a local-only tool.
func (d Deps) stateConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	states, err := d.Svc.Snapshot(ctx)
	if err != nil {
		d.Log.ErrorContext(ctx, "state endpoint", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(states))
	for _, c := range states {
		out = append(out, stateConversation(c))
	}
	writeJSON(ctx, d.Log, w, http.StatusOK, map[string]any{"conversations": out})
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
