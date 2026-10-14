package web_test

import (
	"net/http"
	"net/url"
	"testing"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestInspectorSnapshotsSaveDiffRestore(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)

	_, empty, _ := b.get("/ui/inspector/snapshots")
	contains(t, empty, `data-testid="snapshot-form"`, "No snapshots yet")

	_, saved, _ := b.post("/ui/inspector/snapshots", url.Values{"name": {"base"}})
	contains(t, saved, `data-snapshot="base"`, "Saved snapshot base.")
	_, dup, _ := b.post("/ui/inspector/snapshots", url.Values{"name": {"base"}})
	contains(t, dup, `data-testid="snapshot-error"`, "name exists")
	_, bad, _ := b.post("/ui/inspector/snapshots", url.Values{"name": {"a b"}})
	contains(t, bad, "name must be")

	if _, err := app.Service.PostMessage(t.Context(), w.human, service.PostOpts{Channel: w.general, Content: service.Content{Text: "inspector diff me"}}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	status, diff, _ := b.get("/ui/inspector/diff?from=base")
	if status != http.StatusOK {
		t.Fatalf("diff status = %d", status)
	}
	contains(t, diff, "messages: &#43;1 -0 ~0", "inspector diff me")
	_, missing, _ := b.get("/ui/inspector/diff?from=ghost")
	contains(t, missing, `data-testid="diff-error"`, "no such snapshot")

	_, restored, _ := b.post("/ui/inspector/snapshots/base/restore", nil)
	contains(t, restored, "Restored base")
	_, after, _ := b.get("/ui/inspector/diff?from=base")
	contains(t, after, "messages: &#43;0 -0 ~0")
}
