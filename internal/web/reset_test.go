package web_test

import (
	"net/http"
	"net/url"
	"testing"

	"localslack/internal/control"
	"localslack/internal/storetest"
)

func TestActingAsFallsBackAfterResetRemovesTheUser(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)
	b.post("/ui/as", url.Values{"user": {w.other.ID}})
	if _, err := app.Components.Control.Reset(t.Context(), control.ResetOpts{Seed: "someone-else"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	w2 := setup(t, app) // the new first human; the cookie still names the old user
	if w2.human.ID == w.other.ID {
		t.Fatalf("test setup: reset kept user %s", w.other.ID)
	}

	status, page, _ := b.get("/ui/c/" + w2.general)

	if status != http.StatusOK {
		t.Fatalf("channel page after reset = %d, want 200 (stale cookie must fall back)", status)
	}
	contains(t, page, w2.human.Name)
	if status, _, _ := b.post("/ui/c/"+w2.general+"/post", url.Values{"text": {"still works"}}); status >= http.StatusBadRequest {
		t.Errorf("post with stale cookie = %d, want success as the fallback user", status)
	}
}
