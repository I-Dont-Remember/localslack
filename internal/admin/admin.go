// Package admin serves the /_localslack control API. One file per feature;
// each exposes mountX(r, d). It must not import api, web or store.
package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/faults"
	"localslack/internal/service"
	"localslack/internal/socket"
)

// Deps are the admin routes' collaborators. Svc may be nil (health only).
type Deps struct {
	Log *slog.Logger
	Svc *service.Service
	// Optional components; nil when unwired. Streams read the one they own.
	Calls   *calllog.Log
	Faults  *faults.Set
	Events  events.Dispatcher
	Control *control.Control
	Socket  *socket.Hub
}

// Mount registers every /_localslack route.
func Mount(r chi.Router, d Deps) {
	mountHealth(r, d)
	mountState(r, d)
	mountApps(r, d)
	mountEvents(r, d)
	mountCalls(r, d)
	mountFaults(r, d)
	mountReset(r, d)
	mountSnapshots(r, d)
	mountSocket(r, d)
}

// writeError emits the /_localslack error shape: {"error":{"code","message"}}.
func writeError(ctx context.Context, log *slog.Logger, w http.ResponseWriter, status int, code, message string) {
	respond.Error(ctx, log, w, status, code, message, nil)
}

// writeNoContent answers 204 with no body (delete/clear).
func writeNoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// queryOK is the calls filter parameter; a query key, not a response field.
const queryOK = "ok"

// healthyStatus is the documented /health "status" value, not an ok wrapper.
const healthyStatus = "ok"

// maxBodyBytes caps every /_localslack request body.
const maxBodyBytes = 1 << 20

// capped limits r.Body; reading past the cap fails with *http.MaxBytesError.
func capped(w http.ResponseWriter, r *http.Request) io.Reader {
	return http.MaxBytesReader(w, r.Body, maxBodyBytes)
}

// writeTooLarge answers 413 and reports true when err is a body-cap overflow.
func writeTooLarge(ctx context.Context, log *slog.Logger, w http.ResponseWriter, err error) bool {
	var big *http.MaxBytesError
	if !errors.As(err, &big) {
		return false
	}
	writeError(ctx, log, w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the size limit")
	return true
}
