package web

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"localslack/internal/service"
)

func (w *Web) channelPage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	d, err := w.page(ctx, actor, r.URL.Path, chi.URLParam(r, "id"))
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	w.render(rw, r, "page", d)
}

func (w *Web) channelMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	who, err := w.whoIs(ctx)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	msgs, err := w.channelMessagesView(ctx, actor, chi.URLParam(r, "id"), who)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	w.render(rw, r, "messages", msgs)
}

// post stores the form's text (blank is ignored) then re-renders via again.
func (w *Web) post(rw http.ResponseWriter, r *http.Request, threadTS string, again http.HandlerFunc) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	if text := strings.TrimSpace(r.PostForm.Get("text")); text != "" {
		// The UI actor joins a public channel by posting in it (the API has no join in this tier).
		if err := w.svc.EnsureMember(ctx, actor, chi.URLParam(r, "id")); err != nil {
			w.fail(rw, r, err)
			return
		}
		if _, err := w.svc.PostMessage(ctx, actor, service.PostOpts{Channel: chi.URLParam(r, "id"), ThreadTS: threadTS, Content: service.Content{Text: text}}); err != nil {
			w.fail(rw, r, err)
			return
		}
	}
	again(rw, r)
}

func (w *Web) channelPost(rw http.ResponseWriter, r *http.Request) {
	w.post(rw, r, "", w.channelMessages)
}

func (w *Web) threadPost(rw http.ResponseWriter, r *http.Request) {
	w.post(rw, r, chi.URLParam(r, "ts"), w.threadMessages)
}

func (w *Web) threadPage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	d, err := w.page(ctx, actor, r.URL.Path, id)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	who, err := w.whoIs(ctx)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	if d.Thread, err = w.threadMessagesView(ctx, actor, id, chi.URLParam(r, "ts"), who); err != nil {
		w.fail(rw, r, err)
		return
	}
	w.render(rw, r, "page", d)
}

func (w *Web) threadMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	who, err := w.whoIs(ctx)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	t, err := w.threadMessagesView(ctx, actor, chi.URLParam(r, "id"), chi.URLParam(r, "ts"), who)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	w.render(rw, r, "messages", t.Messages)
}

// switcher is the quick-switcher: channels link, people open a DM via POST.
func (w *Web) switcher(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var items []switchItem
	if q != "" {
		actor, err := w.actor(ctx, r)
		if err != nil {
			w.fail(rw, r, err)
			return
		}
		chans, err := w.svc.ListAll(ctx, actor, []string{"public_channel", "private_channel"}, true)
		if err != nil {
			w.fail(rw, r, err)
			return
		}
		users, err := w.svc.Users(ctx)
		if err != nil {
			w.fail(rw, r, err)
			return
		}
		items = matches(q, chans, users)
	}
	w.render(rw, r, "switcher", items)
}

func matches(q string, chans []service.Conversation, users []service.User) []switchItem {
	var items []switchItem
	for _, c := range chans {
		if strings.Contains(strings.ToLower(c.Name), q) {
			items = append(items, switchItem{Href: "/ui/c/" + c.ID, Label: "#" + c.Name})
		}
	}
	for _, u := range users {
		if strings.Contains(strings.ToLower(u.Name), q) {
			items = append(items, switchItem{UserID: u.ID, Label: "@" + u.Name})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func (w *Web) openDM(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, err := w.actor(ctx, r)
	if err != nil {
		w.fail(rw, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "bad form", http.StatusBadRequest)
		return
	}
	conv, err := w.svc.OpenDM(ctx, actor, []string{r.PostForm.Get("user")})
	if err != nil {
		if errors.Is(err, service.ErrNoUsers) {
			err = service.ErrUserNotFound
		}
		w.fail(rw, r, err)
		return
	}
	rw.Header().Set("HX-Redirect", "/ui/c/"+conv.ID)
	rw.WriteHeader(http.StatusOK)
}
