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

	"localslack/internal/calllog"
	"localslack/internal/constants"
	"localslack/internal/storetest"
)

const faultsPath = "/_localslack/faults"

type reply struct {
	StatusCode int
	Header     http.Header
}

func adminDo(t *testing.T, app *storetest.App, verb, path, body string) (reply, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), verb, app.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
	}
	return reply{resp.StatusCode, resp.Header}, out
}

// slack calls auth.test (a cheap real method) with the given token.
func slack(t *testing.T, app *storetest.App, token string) (reply, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL+"/api/auth.test", strings.NewReader(url.Values{}.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeForm)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return reply{resp.StatusCode, resp.Header}, out
}

func botTok() string { return "xoxb-" + "faults" }

func TestFaultsCRUD(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)

	resp, got := adminDo(t, app, http.MethodPost, faultsPath, `{"method":"auth.test","nth":3,"action":{"status":429,"retry_after":2}}`)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("Location") != faultsPath+"/f_1" {
		t.Errorf("create: status %d location %q, want 201 %s/f_1", resp.StatusCode, resp.Header.Get("Location"), faultsPath)
	}
	want := map[string]any{
		"id": "f_1", "method": "auth.test", "nth": float64(3), "seen": float64(0), "fired": float64(0), "spent": false,
		"action": map[string]any{"status": float64(429), "retry_after": float64(2)},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("created rule (-want +got):\n%s", diff)
	}

	_, list := adminDo(t, app, http.MethodGet, faultsPath, "")
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list = %v, want one item", list)
	}

	resp, _ = adminDo(t, app, http.MethodDelete, faultsPath+"/f_1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", resp.StatusCode)
	}
	resp, body := adminDo(t, app, http.MethodDelete, faultsPath+"/f_1", "")
	errObj, _ := body["error"].(map[string]any)
	if resp.StatusCode != http.StatusNotFound || errObj["code"] != "not_found" {
		t.Errorf("second delete = %d %v, want 404 not_found", resp.StatusCode, body)
	}

	adminDo(t, app, http.MethodPost, faultsPath, `{"method":"*","action":{"status":500}}`)
	resp, _ = adminDo(t, app, http.MethodDelete, faultsPath, "")
	_, list = adminDo(t, app, http.MethodGet, faultsPath, "")
	if resp.StatusCode != http.StatusNoContent || len(list["items"].([]any)) != 0 {
		t.Errorf("delete all: status %d list %v, want 204 and empty", resp.StatusCode, list)
	}
	if list["items"] == nil {
		t.Error("empty list items must be [] not null")
	}
}

func TestFaultsCreateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		wantPtr    string
	}{
		{"malformed json", `{`, 400, "invalid_json", ""},
		{"unknown field", `{"method":"m","bogus":1,"action":{"status":500}}`, 422, "invalid_fault", "/bogus"},
		{"unknown action field", `{"method":"m","action":{"status":500,"nope":1}}`, 422, "invalid_fault", "/action/nope"},
		{"bad status", `{"method":"m","action":{"status":302}}`, 422, "invalid_fault", "/action/status"},
		{"missing method", `{"action":{"status":500}}`, 422, "invalid_fault", "/method"},
		{"delay too long", `{"method":"m","action":{"delay_ms":60001}}`, 422, "invalid_fault", "/action/delay_ms"},
		{"wrong type", `{"method":"m","nth":"3","action":{"status":500}}`, 422, "invalid_fault", "/nth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)

			resp, body := adminDo(t, app, http.MethodPost, faultsPath, tt.body)

			errObj, _ := body["error"].(map[string]any)
			if resp.StatusCode != tt.wantStatus || errObj["code"] != tt.wantCode {
				t.Fatalf("got %d %v, want %d %s", resp.StatusCode, body, tt.wantStatus, tt.wantCode)
			}
			if tt.wantPtr == "" {
				return
			}
			details, _ := errObj["details"].([]any)
			found := false
			for _, d := range details {
				found = found || d.(map[string]any)["pointer"] == tt.wantPtr
			}
			if !found {
				t.Errorf("details = %v, want pointer %s", details, tt.wantPtr)
			}
		})
	}
}

func TestFaultBehaviorOverHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rule        string
		token       string
		wantStatus  int
		wantError   string // "" = real call ok
		wantRetry   string
		wantFaulted bool
	}{
		{"429", `{"method":"auth.test","action":{"status":429,"retry_after":2}}`, botTok(), 429, "ratelimited", "2", true},
		{"429 default retry", `{"method":"auth.test","action":{"status":429}}`, botTok(), 429, "ratelimited", "1", true},
		{"500", `{"method":"auth.test","action":{"status":500}}`, botTok(), 500, "internal_error", "", true},
		{"500 body", `{"method":"auth.test","action":{"status":502,"body":{"ok":false,"error":"boom"}}}`, botTok(), 502, "boom", "", true},
		{"200 error", `{"method":"auth.test","action":{"error":"fatal_error"}}`, botTok(), 200, "fatal_error", "", true},
		{"wildcard", `{"method":"*","action":{"status":503}}`, botTok(), 503, "internal_error", "", true},
		{"bad token still faulted", `{"method":"auth.test","action":{"status":500}}`, "nope", 500, "internal_error", "", true},
		{"delay only runs real method", `{"method":"auth.test","action":{"delay_ms":5}}`, botTok(), 200, "", "", true},
		{"other method unaffected", `{"method":"chat.postMessage","action":{"status":500}}`, botTok(), 200, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)
			adminDo(t, app, http.MethodPost, faultsPath, tt.rule)

			resp, body := slack(t, app, tt.token)

			gotErr, _ := body["error"].(string)
			if resp.StatusCode != tt.wantStatus || gotErr != tt.wantError || resp.Header.Get("Retry-After") != tt.wantRetry {
				t.Errorf("got %d error=%q Retry-After=%q, want %d %q %q (body %v)",
					resp.StatusCode, gotErr, resp.Header.Get("Retry-After"), tt.wantStatus, tt.wantError, tt.wantRetry, body)
			}
			res := app.Components.CallLog.Query(t.Context(), calllog.Query{Method: "auth.test"})
			if len(res.Calls) != 1 {
				t.Fatalf("logged calls = %d, want 1", len(res.Calls))
			}
			if got := res.Calls[0].FaultID == "f_1"; got != tt.wantFaulted {
				t.Errorf("fault_id = %q, faulted want %v", res.Calls[0].FaultID, tt.wantFaulted)
			}
		})
	}
}

func TestFaultNthAffectsOnlyThird(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	adminDo(t, app, http.MethodPost, faultsPath, `{"method":"auth.test","nth":3,"action":{"status":500}}`)

	var got []int
	for range 5 {
		resp, _ := slack(t, app, botTok())
		got = append(got, resp.StatusCode)
	}

	if diff := cmp.Diff([]int{200, 200, 500, 200, 200}, got); diff != "" {
		t.Errorf("statuses (-want +got):\n%s", diff)
	}
	_, list := adminDo(t, app, http.MethodGet, faultsPath, "")
	rule := list["items"].([]any)[0].(map[string]any)
	if rule["seen"] != float64(3) || rule["fired"] != float64(1) || rule["spent"] != true {
		t.Errorf("rule = %v, want seen=3 fired=1 spent (spent rules stop counting)", rule)
	}
}

func TestFaultDelaySlowsCall(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	adminDo(t, app, http.MethodPost, faultsPath, `{"method":"auth.test","action":{"delay_ms":150}}`)

	start := time.Now()
	resp, body := slack(t, app, botTok())

	if took := time.Since(start); took < 150*time.Millisecond || resp.StatusCode != 200 || body["ok"] != true {
		t.Errorf("took %v status %d body %v, want >=150ms and the real ok response", took, resp.StatusCode, body)
	}
}
