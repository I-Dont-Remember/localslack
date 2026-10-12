package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestUIAccess(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)
	ctx := t.Context()
	// The UI actor is w.human; channels made by w.other have no human member.
	pub, err := app.Service.CreateChannel(ctx, w.other, "ui-public")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	priv, err := app.Service.CreateChannel(ctx, w.other, "ui-private", service.CreatePrivate())
	if err != nil {
		t.Fatalf("create private: %v", err)
	}
	mine, err := app.Service.CreateChannel(ctx, w.human, "ui-mine", service.CreatePrivate())
	if err != nil {
		t.Fatalf("create mine: %v", err)
	}

	status, _, _ := b.post("/ui/c/"+pub.ID+"/post", url.Values{"text": {"auto join"}})
	if status != http.StatusOK {
		t.Errorf("post to public non-member channel status = %d, want 200 (UI auto-joins)", status)
	}
	if c, err := app.Service.Conversation(ctx, w.human, pub.ID); err != nil || !c.IsMember {
		t.Errorf("after UI post: %+v, %v; want member", c, err)
	}

	status, _, _ = b.post("/ui/c/"+priv.ID+"/post", url.Values{"text": {"nope"}})
	if status != http.StatusNotFound {
		t.Errorf("post to foreign private channel status = %d, want 404", status)
	}
	if status, _, _ := b.get("/ui/c/" + priv.ID); status != http.StatusNotFound {
		t.Errorf("view foreign private channel status = %d, want 404", status)
	}

	_, page, _ := b.get("/ui/c/" + mine.ID)
	contains(t, page, `data-testid="private-lock"`, "ui-mine")
	if _, page, _ := b.get("/ui/c/" + pub.ID); strings.Contains(page, "ui-private") {
		t.Errorf("sidebar leaks a private channel the actor is not in")
	}
}
