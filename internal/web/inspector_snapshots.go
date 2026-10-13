package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/control"
	"localslack/internal/scenario"
)

// SnapshotStore is the in-memory snapshot facility the inspector drives (*control.Control).
type SnapshotStore interface {
	CreateSnapshot(ctx context.Context, name string) (control.SnapshotInfo, error)
	Snapshots() []control.SnapshotInfo
	Restore(ctx context.Context, name string) (control.RestoreResult, error)
	Diff(ctx context.Context, from, to string) (scenario.Delta, error)
}

type snapshotRow struct {
	Name, Created                             string
	Users, Channels, DMs, Messages, Reactions int
}

type snapshotsView struct {
	Missing       bool
	Error, Notice string
	Rows          []snapshotRow
}

type diffView struct {
	Missing bool
	Error   string
	From    string
	To      string
	Summary []string
	JSON    string
}

func (w *Web) snapshotsView(notice, errMsg string) snapshotsView {
	if w.insp.Snapshots == nil {
		return snapshotsView{Missing: true}
	}
	v := snapshotsView{Notice: notice, Error: errMsg}
	for _, s := range w.insp.Snapshots.Snapshots() {
		v.Rows = append(v.Rows, snapshotRow{
			Name: s.Name, Created: s.Created.Format("15:04:05"), Users: s.Counts.Users, Channels: s.Counts.Channels,
			DMs: s.Counts.DMs, Messages: s.Counts.Messages, Reactions: s.Counts.Reactions,
		})
	}
	return v
}

func snapshotErrMsg(err error) string {
	switch {
	case errors.Is(err, control.ErrInvalidSnapshotName):
		return "name must be 1-64 letters, numerals, '.', '_' or '-' and not \"current\""
	case errors.Is(err, control.ErrSnapshotExists):
		return "a snapshot with that name exists"
	case errors.Is(err, control.ErrSnapshotLimit):
		return "too many snapshots"
	case errors.Is(err, control.ErrSnapshotNotFound):
		return "no such snapshot"
	case errors.Is(err, control.ErrResetInProgress):
		return "a reset or restore is running; retry when it finishes"
	}
	return "failed: " + err.Error()
}

func (w *Web) inspectorSnapshots(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-snapshots", w.snapshotsView("", ""))
}

func (w *Web) inspectorCreateSnapshot(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Snapshots == nil {
		w.render(rw, r, "inspector-snapshots", snapshotsView{Missing: true})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	info, err := w.insp.Snapshots.CreateSnapshot(r.Context(), strings.TrimSpace(r.PostForm.Get("name")))
	if err != nil {
		w.render(rw, r, "inspector-snapshots", w.snapshotsView("", snapshotErrMsg(err)))
		return
	}
	w.render(rw, r, "inspector-snapshots", w.snapshotsView("Saved snapshot "+info.Name+".", ""))
}

func (w *Web) inspectorRestoreSnapshot(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Snapshots == nil {
		w.render(rw, r, "inspector-snapshots", snapshotsView{Missing: true})
		return
	}
	res, err := w.insp.Snapshots.Restore(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		w.render(rw, r, "inspector-snapshots", w.snapshotsView("", snapshotErrMsg(err)))
		return
	}
	w.render(rw, r, "inspector-snapshots", w.snapshotsView(fmt.Sprintf("Restored %s in %d ms: %d users, %d channels, %d messages.",
		res.Name, res.TookMS, res.Counts.Users, res.Counts.Channels, res.Counts.Messages), ""))
}

func (w *Web) inspectorDiff(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Snapshots == nil {
		w.render(rw, r, "inspector-diff", diffView{Missing: true})
		return
	}
	from, to := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if to == "" {
		to = control.CurrentRef
	}
	if from == "" {
		w.render(rw, r, "inspector-diff", diffView{Error: "pick a snapshot to compare from"})
		return
	}
	d, err := w.insp.Snapshots.Diff(r.Context(), from, to)
	if err != nil {
		w.render(rw, r, "inspector-diff", diffView{Error: snapshotErrMsg(err)})
		return
	}
	pretty, _ := json.MarshalIndent(d, "", "  ")
	w.render(rw, r, "inspector-diff", diffView{From: from, To: to, Summary: diffSummary(d), JSON: string(pretty)})
}

func diffSummary(d scenario.Delta) []string {
	line := func(name string, added, removed, changed int) string {
		return fmt.Sprintf("%s: +%d -%d ~%d", name, added, removed, changed)
	}
	return []string{
		line("users", len(d.Users.Added), len(d.Users.Removed), len(d.Users.Changed)),
		line("conversations", len(d.Conversations.Added), len(d.Conversations.Removed), len(d.Conversations.Changed)),
		line("members", len(d.Members.Added), len(d.Members.Removed), 0),
		line("messages", len(d.Messages.Added), len(d.Messages.Removed), len(d.Messages.Changed)),
		line("reactions", len(d.Reactions.Added), len(d.Reactions.Removed), 0),
	}
}
