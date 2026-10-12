package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

// callsPage is the list envelope of GET /_localslack/calls.
type callsPage struct {
	Items []struct {
		Seq    int64          `json:"seq"`
		Method string         `json:"method"`
		OK     bool           `json:"ok"`
		Error  string         `json:"error"`
		Params map[string]any `json:"params"`
		Actor  struct {
			TokenKind string `json:"token_kind"`
		} `json:"actor"`
	} `json:"items"`
	NextSince  int64 `json:"next_since"`
	Generation int64 `json:"generation"`
}

func do(t *testing.T, method, u string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, u, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, b
}

// slackCall makes a Web API call, which the call log records.
func slackCall(t *testing.T, base, method string, form url.Values) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/api/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer xoxb-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("slack call: %v", err)
	}
	_ = resp.Body.Close() // result is asserted through the call log
}

func listCalls(t *testing.T, base, query string) (int, callsPage) {
	t.Helper()
	status, b := do(t, http.MethodGet, base+"/_localslack/calls?"+query)
	var page callsPage
	if status == http.StatusOK {
		if err := json.Unmarshal(b, &page); err != nil {
			t.Fatalf("decode %s: %v", b, err)
		}
	}
	return status, page
}

func methodsOf(p callsPage) []string {
	out := []string{}
	for _, c := range p.Items {
		out = append(out, c.Method)
	}
	return out
}

func TestCallsListFilters(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	slackCall(t, app.URL, "auth.test", nil)
	slackCall(t, app.URL, "chat.postMessage", url.Values{"channel": {"CNOPE"}, "text": {"x"}})
	slackCall(t, app.URL, "no.such_method", nil)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"all, oldest first", "", []string{"auth.test", "chat.postMessage", "no.such_method"}},
		{"method", "method=auth.test", []string{"auth.test"}},
		{"channel matches params.channel", "channel=CNOPE", []string{"chat.postMessage"}},
		{"ok=false", "ok=false", []string{"chat.postMessage", "no.such_method"}},
		{"ok=true", "ok=true", []string{"auth.test"}},
		{"error", "error=channel_not_found", []string{"chat.postMessage"}},
		{"since is exclusive", "since=2", []string{"no.such_method"}},
		{"limit", "limit=2", []string{"auth.test", "chat.postMessage"}},
		{"min already met returns immediately", "min=1&timeout_ms=5000", []string{"auth.test", "chat.postMessage", "no.such_method"}},
		{"no match is an empty list, not null", "method=nope", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, page := listCalls(t, app.URL, tt.query)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if diff := cmp.Diff(tt.want, methodsOf(page)); diff != "" {
				t.Errorf("methods (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCallsListNextSince(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	slackCall(t, app.URL, "auth.test", nil)
	slackCall(t, app.URL, "auth.test", nil)

	_, all := listCalls(t, app.URL, "")
	_, none := listCalls(t, app.URL, "since=2")

	if all.NextSince != 2 || none.NextSince != 2 || len(none.Items) != 0 {
		t.Errorf("next_since all=%d none=%d items=%d, want 2, 2 (echoed), 0", all.NextSince, none.NextSince, len(none.Items))
	}
}

func TestCallsListRejectsBadQuery(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	for _, q := range []string{"ok=maybe", "since=abc", "limit=-1", "min=x", "timeout_ms=1.5", "since=-3"} {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			status, b := do(t, http.MethodGet, app.URL+"/_localslack/calls?"+q)
			var e struct{ Error struct{ Code string } }
			_ = json.Unmarshal(b, &e)
			if status != http.StatusBadRequest || e.Error.Code != "invalid_query" {
				t.Errorf("%s -> %d %s, want 400 invalid_query", q, status, b)
			}
		})
	}
}

func TestCallsLongPollWakesOnNewCall(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	done := make(chan callsPage, 1)
	go func() {
		_, page := listCalls(t, app.URL, "min=1&timeout_ms=10000")
		done <- page
	}()
	for app.Components.CallLog.Waiters() == 0 { // parked
		time.Sleep(time.Millisecond)
	}
	slackCall(t, app.URL, "auth.test", nil)

	select {
	case page := <-done:
		if diff := cmp.Diff([]string{"auth.test"}, methodsOf(page)); diff != "" {
			t.Errorf("methods (-want +got):\n%s", diff)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long poll did not wake on a new call")
	}
}

func TestCallsLongPollTimesOutWithEmptyList(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	status, page := listCalls(t, app.URL, "min=1&timeout_ms=20")
	if status != http.StatusOK || len(page.Items) != 0 {
		t.Errorf("timeout poll = %d with %d items, want 200 and none", status, len(page.Items))
	}
}

func TestCallsDeleteClearsAndKeepsSeq(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	slackCall(t, app.URL, "auth.test", nil)
	_, before := listCalls(t, app.URL, "")

	status, body := do(t, http.MethodDelete, app.URL+"/_localslack/calls")
	slackCall(t, app.URL, "auth.test", nil)
	_, after := listCalls(t, app.URL, "")

	if status != http.StatusNoContent || len(body) != 0 {
		t.Errorf("DELETE = %d %q, want 204 empty", status, body)
	}
	if len(after.Items) != 1 || after.Items[0].Seq <= before.Items[0].Seq || after.Generation <= before.Generation {
		t.Errorf("after delete: items %+v gen %d (before gen %d), want 1 new item with greater seq and generation", after.Items, after.Generation, before.Generation)
	}
}

func TestLocalslackAndUIRoutesAreNotLogged(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	do(t, http.MethodGet, app.URL+"/_localslack/health")
	do(t, http.MethodGet, app.URL+"/ui/")

	if _, page := listCalls(t, app.URL, ""); len(page.Items) != 0 {
		t.Errorf("logged %v, want nothing from /_localslack or /ui", methodsOf(page))
	}
}

func TestCallsRedactsToken(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	slackCall(t, app.URL, "auth.test", url.Values{"token": {"xoxb-secret-value"}})

	_, page := listCalls(t, app.URL, "")
	if got := page.Items[0].Params["token"]; got != "[redacted]" {
		t.Errorf("params.token = %v, want [redacted]", got)
	}
}
