package web_test

import (
	"net/http"
	"net/url"
	"testing"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestReactFromUI(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	w := setup(t, app)
	b := newBrowser(t, app)
	root, err := app.Service.PostMessage(t.Context(), w.other, service.PostOpts{Channel: w.general, Content: service.Content{Text: "root msg"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	reply, err := app.Service.PostMessage(t.Context(), w.other, service.PostOpts{Channel: w.general, ThreadTS: root.TS, Content: service.Content{Text: "reply msg"}})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	react := func(path, ts, name string) (int, string) {
		status, body, _ := b.post(path, url.Values{"message": {ts}, "name": {name}})
		return status, body
	}

	status, body := react("/ui/c/"+w.general+"/react", root.TS, "eyes")
	if status != http.StatusOK {
		t.Fatalf("react status = %d, body %s", status, body)
	}
	contains(t, body, `data-testid="reaction"`, `data-name="eyes"`, "👀 1", `data-testid="react-button"`, `data-testid="react-option-tada"`)

	if status, _ := react("/ui/c/"+w.general+"/react", root.TS, "eyes"); status != http.StatusOK {
		t.Errorf("repeat react status = %d, want 200 (already reacted is a no-op)", status)
	}
	status, body = react("/ui/c/"+w.general+"/t/"+root.TS+"/react", reply.TS, "tada")
	if status != http.StatusOK {
		t.Fatalf("thread react status = %d, body %s", status, body)
	}
	contains(t, body, `data-name="tada"`, "reply msg")

	for _, tt := range []struct {
		name, ts, emoji string
		want            int
	}{
		{"invalid name", root.TS, "not valid!", http.StatusBadRequest},
		{"no message", "1.000001", "eyes", http.StatusNotFound},
	} {
		if status, _ := react("/ui/c/"+w.general+"/react", tt.ts, tt.emoji); status != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, status, tt.want)
		}
	}

	hist, err := app.Service.History(t.Context(), w.human, w.general, service.PageOpts{Limit: 1, Oldest: root.TS, Inclusive: true})
	if err != nil || len(hist.Items) != 1 || len(hist.Items[0].Reactions) != 1 || hist.Items[0].Reactions[0].Count != 1 {
		t.Errorf("history = %+v, %v; want the root with one eyes reaction by the UI actor (counted once)", hist.Items, err)
	}
	_, page, _ := b.get("/ui/c/" + w.general)
	contains(t, page, `data-name="eyes"`)
}
