package admin_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

// doReq sends body (any JSON value, or a string sent verbatim) and decodes the JSON reply.
func doReq(t *testing.T, method, url, contentType string, body any) (int, http.Header, map[string]any) {
	t.Helper()
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode %s %s reply %q: %v", method, url, raw, err)
		}
	}
	return resp.StatusCode, resp.Header, out
}

// echoApp is a Slack app that answers url_verification (unless failVerify is set).
type echoApp struct {
	*httptest.Server
	failVerify atomic.Bool
}

func newEchoApp(t *testing.T) *echoApp {
	t.Helper()
	a := &echoApp{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Type, Challenge string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if a.failVerify.Load() || b.Type != "url_verification" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", constants.ContentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": b.Challenge})
	}))
	t.Cleanup(a.Close)
	return a
}

func manifest(name, requestURL string, events ...string) map[string]any {
	return map[string]any{
		"display_information": map[string]any{"name": name},
		"features":            map[string]any{"bot_user": map[string]any{"display_name": name}},
		"settings": map[string]any{"event_subscriptions": map[string]any{
			"request_url": requestURL, "bot_events": events,
		}},
	}
}

func sub(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func TestPostAppRawJSONManifest(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newEchoApp(t)
	status, hdr, got := doReq(t, http.MethodPost, app.URL+"/_localslack/apps", constants.ContentTypeJSON,
		manifest("Raw App", fake.URL+"/slack/events", "app_mention"))

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %v", status, got)
	}
	id, _ := got["id"].(string)
	if !strings.HasPrefix(id, "A") || hdr.Get("Location") != "/_localslack/apps/"+id {
		t.Errorf("id = %q, Location = %q", id, hdr.Get("Location"))
	}
	creds := sub(got, "credentials")
	for _, k := range []string{"signing_secret", "bot_token", "app_token", "verification_token"} {
		if s, _ := creds[k].(string); s == "" {
			t.Errorf("credentials.%s is empty", k)
		}
	}
	if got["delivery"] != "ready" || got["verified"] != true || got["name"] != "Raw App" {
		t.Errorf("app = %v, want ready + verified", got)
	}
	subs := sub(got, "event_subscriptions")
	if subs["effective_request_url"] != fake.URL+"/slack/events" || subs["manifest_request_url"] != fake.URL+"/slack/events" {
		t.Errorf("event_subscriptions = %v", subs)
	}
	if w, ok := got["warnings"].([]any); !ok || len(w) != 0 {
		t.Errorf("warnings = %v, want []", got["warnings"])
	}
	for _, k := range []string{"bot_user_id", "bot_id", "deliver_to", "socket_mode_enabled", "scopes"} {
		if _, ok := got[k]; !ok {
			t.Errorf("app lacks %q", k)
		}
	}
}

func TestPostAppYAMLManifest(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newEchoApp(t)
	doc := "display_information:\n  name: Yaml App\nsettings:\n  event_subscriptions:\n    request_url: " + fake.URL + "/e\n    bot_events: [message.channels]\n"
	status, _, got := doReq(t, http.MethodPost, app.URL+"/_localslack/apps", "application/yaml", doc)
	if status != http.StatusCreated || got["name"] != "Yaml App" {
		t.Fatalf("status = %d body %v", status, got)
	}
	subs := sub(got, "event_subscriptions")
	if diff := cmp.Diff([]any{"message.channels"}, subs["bot_events"]); diff != "" {
		t.Errorf("bot_events (-want +got):\n%s", diff)
	}
}

func TestPostAppEnvelopeWithOverrides(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newEchoApp(t)
	status, _, got := doReq(t, http.MethodPost, app.URL+"/_localslack/apps", constants.ContentTypeJSON, map[string]any{
		"manifest":       manifest("Env App", "https://example.invalid/hooks/slack", "app_mention"),
		"deliver_to":     fake.URL,
		"signing_secret": "my-secret",
		"bot_token":      "xo" + "xb-mine",
		"app_token":      "xa" + "pp-mine",
		"app_id":         "A0MINE0001",
		"verify":         false,
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d; body %v", status, got)
	}
	creds := sub(got, "credentials")
	if got["id"] != "A0MINE0001" || creds["signing_secret"] != "my-secret" || creds["bot_token"] != "xo"+"xb-mine" || creds["app_token"] != "xa"+"pp-mine" {
		t.Errorf("overrides not honored: %v", got)
	}
	if got["deliver_to"] != fake.URL || got["delivery"] != "ready" {
		t.Errorf("deliver_to/delivery = %v / %v", got["deliver_to"], got["delivery"])
	}
	if subs := sub(got, "event_subscriptions"); subs["effective_request_url"] != fake.URL+"/hooks/slack" {
		t.Errorf("host-only deliver_to must keep the manifest path; effective = %v", subs["effective_request_url"])
	}
	if got["verified"] != false {
		t.Errorf("verify:false must skip the handshake, verified = %v", got["verified"])
	}
}

func TestPostAppErrors(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	url := app.URL + "/_localslack/apps"
	doReq(t, http.MethodPost, url, constants.ContentTypeJSON, map[string]any{"manifest": manifest("Dup", ""), "verify": false})
	doReq(t, http.MethodPost, url, constants.ContentTypeJSON, map[string]any{"manifest": manifest("Victim", ""), "bot_token": "xo" + "xb-stolen", "verify": false})

	tests := []struct {
		name        string
		contentType string
		body        any
		wantStatus  int
		wantCode    string
		wantPointer string
	}{
		{"invalid manifest", constants.ContentTypeJSON, map[string]any{"display_information": map[string]any{}}, 422, "invalid_manifest", "/display_information/name"},
		{"malformed json", constants.ContentTypeJSON, "{", 400, "malformed_request", ""},
		{"malformed yaml", "application/yaml", "a: [b", 400, "malformed_request", ""},
		{"duplicate", constants.ContentTypeJSON, map[string]any{"manifest": manifest("Dup", ""), "verify": false}, 409, "app_exists", ""},
		{"token reused by another app", constants.ContentTypeJSON, map[string]any{"manifest": manifest("Thief", ""), "bot_token": "xo" + "xb-stolen", "verify": false}, 409, "already_exists", ""},
		{"bad deliver_to", constants.ContentTypeJSON, map[string]any{"manifest": manifest("Other", ""), "deliver_to": "nope"}, 422, "invalid_deliver_to", "/deliver_to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, _, got := doReq(t, http.MethodPost, url, tt.contentType, tt.body)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %v", status, tt.wantStatus, got)
			}
			e := sub(got, "error")
			if e["code"] != tt.wantCode || e["message"] == "" {
				t.Errorf("error = %v, want code %s", e, tt.wantCode)
			}
			if tt.wantPointer != "" {
				d, _ := e["details"].([]any)
				if len(d) == 0 || d[0].(map[string]any)["pointer"] != tt.wantPointer {
					t.Errorf("details = %v, want pointer %s", e["details"], tt.wantPointer)
				}
			}
		})
	}
}

func TestAppDeliveryBlockedAndWarnings(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	status, _, got := doReq(t, http.MethodPost, app.URL+"/_localslack/apps", constants.ContentTypeJSON,
		manifest("Remote", "https://example.invalid/slack/events", "app_mention", "team_join"))
	if status != http.StatusCreated {
		t.Fatalf("status = %d; %v", status, got)
	}
	if got["delivery"] != "blocked" || !strings.Contains(got["blocked_reason"].(string), "not loopback/private") {
		t.Errorf("delivery = %v, reason %v", got["delivery"], got["blocked_reason"])
	}
	if sub(got, "event_subscriptions")["effective_request_url"] != "" || got["verified"] != false {
		t.Errorf("blocked app: %v", got)
	}
	var warnings []string
	for _, w := range got["warnings"].([]any) {
		warnings = append(warnings, w.(string))
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "team_join") || !strings.Contains(joined, "not loopback/private") {
		t.Errorf("warnings = %q, want the unsupported event and the blocked reason", warnings)
	}
}

func TestAppVerifyFailureStillRegisters(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newEchoApp(t)
	fake.failVerify.Store(true)
	status, _, got := doReq(t, http.MethodPost, app.URL+"/_localslack/apps", constants.ContentTypeJSON, manifest("Flaky", fake.URL+"/e", "app_mention"))
	if status != http.StatusCreated || got["verified"] != false || got["delivery"] != "ready" {
		t.Fatalf("status %d app %v, want 201, verified=false, delivery ready (permissive)", status, got)
	}
	if w := got["warnings"].([]any); len(w) == 0 {
		t.Error("failed verification must leave a warning")
	}

	// Fix the app and re-verify through the endpoint.
	fake.failVerify.Store(false)
	status, _, again := doReq(t, http.MethodPost, app.URL+"/_localslack/apps/"+got["id"].(string)+"/verify", "", nil)
	if status != http.StatusOK || again["verified"] != true {
		t.Errorf("verify status %d app %v, want 200 verified", status, again)
	}
	if w := again["warnings"].([]any); len(w) != 0 {
		t.Errorf("warnings after successful verify = %v", w)
	}
}

func TestAppsListGetPatchDelete(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	base := app.URL + "/_localslack/apps"
	_, _, a := doReq(t, http.MethodPost, base, constants.ContentTypeJSON, map[string]any{"manifest": manifest("Alpha", "https://example.invalid/a"), "verify": false})
	_, _, b := doReq(t, http.MethodPost, base, constants.ContentTypeJSON, map[string]any{"manifest": manifest("Beta", ""), "verify": false})
	idA, idB := a["id"].(string), b["id"].(string)

	status, _, list := doReq(t, http.MethodGet, base, "", nil)
	items, _ := list["items"].([]any)
	if status != http.StatusOK || len(items) != 2 {
		t.Fatalf("list status %d items %v", status, list)
	}

	status, _, one := doReq(t, http.MethodGet, base+"/"+idA, "", nil)
	if status != http.StatusOK || one["id"] != idA || one["delivery"] != "blocked" {
		t.Fatalf("get status %d app %v", status, one)
	}
	status, _, missing := doReq(t, http.MethodGet, base+"/A404", "", nil)
	if status != http.StatusNotFound || sub(missing, "error")["code"] != "app_not_found" {
		t.Errorf("get unknown: status %d body %v", status, missing)
	}

	status, _, patched := doReq(t, http.MethodPatch, base+"/"+idA, constants.ContentTypeJSON, map[string]any{"deliver_to": "http://localhost:4000"})
	if status != http.StatusOK || patched["deliver_to"] != "http://localhost:4000" || patched["delivery"] != "ready" {
		t.Fatalf("patch status %d app %v", status, patched)
	}
	if sub(patched, "event_subscriptions")["effective_request_url"] != "http://localhost:4000/a" {
		t.Errorf("effective url = %v", sub(patched, "event_subscriptions")["effective_request_url"])
	}
	if status, _, bad := doReq(t, http.MethodPatch, base+"/"+idA, constants.ContentTypeJSON, map[string]any{"deliver_to": "nope"}); status != 422 || sub(bad, "error")["code"] != "invalid_deliver_to" {
		t.Errorf("patch bad deliver_to: status %d body %v", status, bad)
	}
	if status, _, _ := doReq(t, http.MethodPatch, base+"/A404", constants.ContentTypeJSON, map[string]any{"deliver_to": ""}); status != http.StatusNotFound {
		t.Errorf("patch unknown status = %d", status)
	}
	_, _, cleared := doReq(t, http.MethodPatch, base+"/"+idA, constants.ContentTypeJSON, map[string]any{"deliver_to": ""})
	if cleared["deliver_to"] != "" || cleared["delivery"] != "blocked" {
		t.Errorf("clearing deliver_to: %v", cleared)
	}

	if status, _, _ := doReq(t, http.MethodDelete, base+"/"+idB, "", nil); status != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", status)
	}
	if status, _, _ := doReq(t, http.MethodGet, base+"/"+idB, "", nil); status != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want 404", status)
	}
	if status, _, _ := doReq(t, http.MethodDelete, base+"/"+idB, "", nil); status != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", status)
	}
	if status, _, _ := doReq(t, http.MethodPost, base+"/A404/verify", "", nil); status != http.StatusNotFound {
		t.Errorf("verify unknown status = %d, want 404", status)
	}
}
