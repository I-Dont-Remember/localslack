package web_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"localslack/internal/storetest"
)

func TestInspectorSockets(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	b := newBrowser(t, app)
	a := registerApp(t, app, "Sock", "")

	_, empty, _ := b.get("/ui/inspector/sockets")
	contains(t, empty, "No Socket Mode connections")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(app.URL, "http") + "/_localslack/socket?app_id=" + a.ID + "&ticket=" + app.Components.Socket.Issue(a.ID)
	c, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if _, _, err := c.Read(ctx); err != nil { // hello
		t.Fatalf("read hello: %v", err)
	}

	_, list, _ := b.get("/ui/inspector/sockets")
	contains(t, list, `data-testid="socket-row"`, a.ID, "never", `data-testid="socket-disconnect"`)
	id := app.Components.Socket.Connections()[0].ID

	_, done, _ := b.post("/ui/inspector/sockets/"+id+"/disconnect", url.Values{})
	contains(t, done, "Sent refresh_requested to "+id, "No Socket Mode connections")
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read disconnect: %v", err)
	}
	var got struct{ Type, Reason string }
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := (struct{ Type, Reason string }{"disconnect", "refresh_requested"}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	_, gone, _ := b.post("/ui/inspector/sockets/"+id+"/disconnect", url.Values{})
	contains(t, gone, "already gone")
}
