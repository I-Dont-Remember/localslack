package admin

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"localslack/internal/scenario"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"

	"localslack/internal/service"
)

func mountState(r chi.Router, d Deps) {
	if d.Svc == nil {
		return
	}
	r.Get("/_localslack/state", d.state)
	r.Get("/_localslack/state/conversations", d.stateConversations)
}

// state serves the whole workspace as a scenario fixture (the snapshot format).
func (d Deps) state(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fx, err := d.Svc.Export(ctx)
	if err != nil {
		d.Log.ErrorContext(ctx, "state endpoint", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	respond.JSON(ctx, d.Log, w, http.StatusOK, fx)
}

// stateConversations dumps every conversation with members and messages as a
// thin view of the service Export (so it shares the snapshot's consistency and
// excludes app bot users). Unauthenticated debug endpoint: LocalSlack is local-only.
func (d Deps) stateConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fx, err := d.Svc.Export(ctx)
	if err != nil {
		d.Log.ErrorContext(ctx, "state endpoint", slog.Any("error", err))
		writeError(ctx, d.Log, w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	respond.JSON(ctx, d.Log, w, http.StatusOK, map[string]any{"conversations": stateView(fx)})
}

func stateView(fx scenario.Fixture) []map[string]any {
	msgs := map[string][]map[string]any{}
	replies := map[[2]string]int{}
	for _, m := range fx.Messages {
		if m.ThreadTS != "" {
			replies[[2]string{m.Channel, m.ThreadTS}]++
		}
	}
	for _, m := range fx.Messages {
		thread, count := m.ThreadTS, replies[[2]string{m.Channel, m.TS}]
		if thread == "" && count > 0 {
			thread = m.TS
		}
		msgs[m.Channel] = append(msgs[m.Channel], map[string]any{
			"ts": m.TS, "user": m.User, "text": m.Text, "thread_ts": thread, "reply_count": count,
		})
	}
	out := make([]map[string]any, 0, len(fx.Channels)+len(fx.DMs))
	add := func(id, name, kind string, archived bool, members []string) {
		ms := msgs[id]
		if ms == nil {
			ms = []map[string]any{}
		}
		if members == nil {
			members = []string{}
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "kind": kind, "is_archived": archived, "members": members, "messages": ms,
		})
	}
	for _, c := range fx.Channels {
		add(c.ID, c.Name, service.KindChannel, c.IsArchived, c.Members)
	}
	for _, dm := range fx.DMs {
		kind := service.KindIM
		if len(dm.Members) > 2 {
			kind = service.KindMPIM
		}
		add(dm.ID, "", kind, false, dm.Members)
	}
	slices.SortFunc(out, func(a, b map[string]any) int { return strings.Compare(a["id"].(string), b["id"].(string)) })
	return out
}
