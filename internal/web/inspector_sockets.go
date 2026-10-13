package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/socket"
)

// SocketHub is the Socket Mode connection set the inspector lists and drops (*socket.Hub).
type SocketHub interface {
	Connections() []socket.ConnInfo
	Disconnect(id, reason string, drop bool) (int, error)
}

type socketRow struct {
	ID, App, ConnectedAt, LastAckAt string
	Pending                         int
}

type socketsView struct {
	Missing bool
	Error   string
	Notice  string
	Rows    []socketRow
}

func (w *Web) socketsView(notice, errMsg string) socketsView {
	if w.insp.Sockets == nil {
		return socketsView{Missing: true}
	}
	v := socketsView{Notice: notice, Error: errMsg}
	for _, c := range w.insp.Sockets.Connections() {
		row := socketRow{ID: c.ID, App: c.AppID, ConnectedAt: c.ConnectedAt.Format(time.TimeOnly), LastAckAt: "never", Pending: c.Pending}
		if c.LastAckAt != nil {
			row.LastAckAt = c.LastAckAt.Format(time.TimeOnly)
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

func (w *Web) inspectorSockets(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "inspector-sockets", w.socketsView("", ""))
}

func (w *Web) inspectorSocketDisconnect(rw http.ResponseWriter, r *http.Request) {
	if w.insp.Sockets == nil {
		w.render(rw, r, "inspector-sockets", socketsView{Missing: true})
		return
	}
	id := chi.URLParam(r, "id")
	_, err := w.insp.Sockets.Disconnect(id, socket.ReasonRefreshRequested, false)
	switch {
	case errors.Is(err, socket.ErrNoSuchConnection):
		w.render(rw, r, "inspector-sockets", w.socketsView("", "connection "+id+" is already gone"))
	case err != nil:
		w.render(rw, r, "inspector-sockets", w.socketsView("", "failed: "+err.Error()))
	default:
		w.render(rw, r, "inspector-sockets", w.socketsView("Sent refresh_requested to "+id+".", ""))
	}
}
