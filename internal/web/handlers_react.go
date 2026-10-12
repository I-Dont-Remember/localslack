package web

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"localslack/internal/service"
)

// emoji is a reaction short name and the glyph the UI shows for it.
type emoji struct{ Name, Glyph string }

// pickerEmoji is the small set offered by the react button.
func pickerEmoji() []emoji {
	return []emoji{{"+1", "👍"}, {"heart", "❤️"}, {"smile", "😄"}, {"tada", "🎉"}, {"eyes", "👀"}}
}

// glyphFor renders a reaction name: a known glyph, else the :name: text.
func glyphFor(name string) string {
	for _, e := range pickerEmoji() {
		if e.Name == name {
			return e.Glyph
		}
	}
	return ":" + name + ":"
}

type reactionView struct {
	Name, Glyph string
	Count       int
}

func reactionViews(rs []service.Reaction) []reactionView {
	out := make([]reactionView, 0, len(rs))
	for _, r := range rs {
		out = append(out, reactionView{Name: r.Name, Glyph: glyphFor(r.Name), Count: r.Count})
	}
	return out
}

// reactThen adds the form's reaction as the acting user, then re-renders via again.
// Reacting twice with the same emoji is a no-op, so double clicks stay quiet.
func (w *Web) reactThen(again http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
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
		err = w.svc.AddReaction(ctx, actor, chi.URLParam(r, "id"), r.PostForm.Get("message"), r.PostForm.Get("name"))
		if err != nil && !errors.Is(err, service.ErrAlreadyReacted) {
			w.fail(rw, r, err)
			return
		}
		again(rw, r)
	}
}

func (w *Web) channelReact(rw http.ResponseWriter, r *http.Request) {
	w.reactThen(w.channelMessages)(rw, r)
}

func (w *Web) threadReact(rw http.ResponseWriter, r *http.Request) {
	w.reactThen(w.threadMessages)(rw, r)
}
