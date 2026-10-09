// Package web serves the htmx browser UI over the service layer. It never
// imports api or store; "acting as" is a cookie holding a user id.
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/constants"
	"localslack/internal/service"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/htmx.min.js
var staticFS embed.FS

const (
	actorCookie  = "localslack_user"
	historyLimit = 200
)

// Web is the UI handler set.
type Web struct {
	svc  *service.Service
	log  *slog.Logger
	tmpl *template.Template
}

// New parses templates once at startup so a bad template fails fast.
func New(svc *service.Service, log *slog.Logger) (*Web, error) {
	t, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Web{svc: svc, log: log, tmpl: t}, nil
}

// Mount registers the UI routes.
func (w *Web) Mount(r chi.Router) error {
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}
	r.Handle("/ui/static/*", http.StripPrefix("/ui/static/", http.FileServerFS(static)))
	r.Get("/", w.home)
	r.Get("/ui", w.home)
	r.Get("/ui/c/{id}", w.channelPage)
	r.Get("/ui/c/{id}/messages", w.channelMessages)
	r.Post("/ui/c/{id}/post", w.channelPost)
	r.Get("/ui/c/{id}/t/{ts}", w.threadPage)
	r.Get("/ui/c/{id}/t/{ts}/messages", w.threadMessages)
	r.Post("/ui/c/{id}/t/{ts}/post", w.threadPost)
	r.Get("/ui/switch", w.switcher)
	r.Post("/ui/dm", w.openDM)
	r.Post("/ui/as", w.actAs)
	return nil
}

// actor resolves the acting user from the cookie, defaulting to the first human.
func (w *Web) actor(ctx context.Context, r *http.Request) (service.User, error) {
	if c, err := r.Cookie(actorCookie); err == nil {
		if u, err := w.svc.Authenticate(ctx, "xoxp-"+c.Value); err == nil {
			return u, nil
		}
	}
	users, err := w.svc.Users(ctx)
	if err != nil {
		return service.User{}, err
	}
	for _, u := range users {
		if !u.IsBot {
			return u, nil
		}
	}
	return w.svc.Authenticate(ctx, "xoxb-ui")
}

func (w *Web) render(rw http.ResponseWriter, r *http.Request, name string, data any) {
	var buf bytes.Buffer
	if err := w.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		w.fail(rw, r, err)
		return
	}
	rw.Header().Set("Content-Type", constants.ContentTypeHTML+"; charset=utf-8")
	if _, err := buf.WriteTo(rw); err != nil {
		w.log.ErrorContext(r.Context(), "write response", slog.Any("error", err))
	}
}

// fail maps domain errors to HTTP statuses (the UI's analogue of Slack error codes).
func (w *Web) fail(rw http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrChannelNotFound), errors.Is(err, service.ErrThreadNotFound), errors.Is(err, service.ErrUserNotFound):
		http.Error(rw, "not found", http.StatusNotFound)
	case errors.Is(err, service.ErrIsArchived):
		http.Error(rw, "channel is archived", http.StatusConflict)
	default:
		w.log.ErrorContext(r.Context(), "web error", slog.Any("error", err))
		http.Error(rw, "internal error", http.StatusInternalServerError)
	}
}

func (w *Web) home(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/ui" {
		http.NotFound(rw, r)
		return
	}
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	chans, err := w.svc.ListAll(ctx, actor, []string{"public_channel"}, true)
	if err != nil || len(chans) == 0 {
		w.fail(rw, r, errors.Join(err, errors.New("no channels")))
		return
	}
	target := chans[0].ID
	for _, c := range chans {
		if c.Name == "general" {
			target = c.ID
		}
	}
	http.Redirect(rw, r, "/ui/c/"+target, http.StatusFound)
}

func (w *Web) actAs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	user := r.PostForm.Get("user")
	if _, err := w.svc.Authenticate(ctx, "xoxp-"+user); err != nil {
		http.Error(rw, "unknown user", http.StatusBadRequest)
		return
	}
	http.SetCookie(rw, &http.Cookie{Name: actorCookie, Value: user, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(rw, r, safeNext(r.PostForm.Get("next")), http.StatusSeeOther)
}

// safeNext only allows same-site absolute paths, refusing open redirects. Control
// characters are rejected outright: browsers strip tabs/newlines from URLs, which
// would turn "/\t/evil.example" into "//evil.example".
func safeNext(next string) string {
	for _, c := range next {
		if c < 0x20 || c == 0x7f || c == '\\' {
			return "/"
		}
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return next
}
