package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/control"
	"localslack/internal/scenario"
	"localslack/internal/storetest"
)

type rawReply struct {
	Status int
	Header http.Header
	Body   []byte
}

func rawDo(t *testing.T, app *storetest.App, verb, path, body string) rawReply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), verb, app.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", verb, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return rawReply{resp.StatusCode, resp.Header, raw}
}

func (r rawReply) fixture(t *testing.T) scenario.Fixture {
	t.Helper()
	var fx scenario.Fixture
	if err := json.Unmarshal(r.Body, &fx); err != nil {
		t.Fatalf("decode fixture %q: %v", r.Body, err)
	}
	return fx
}

type apiError struct {
	Error struct {
		Code    string
		Message string
		Details []scenario.Issue
	}
}

func (r rawReply) apiError(t *testing.T) apiError {
	t.Helper()
	var e apiError
	if err := json.Unmarshal(r.Body, &e); err != nil {
		t.Fatalf("decode error %q: %v", r.Body, err)
	}
	return e
}

func postGeneral(t *testing.T, app *storetest.App, text string) string {
	t.Helper()
	ctx := t.Context()
	fx, err := app.Service.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	general := ""
	for _, c := range fx.Channels {
		if c.Name == "general" {
			general = c.ID
		}
	}
	form := url.Values{"channel": {general}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, app.URL+"/api/chat.postMessage", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", constants.ContentTypeForm)
	req.Header.Set("Authorization", "Bearer xoxb-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		OK bool
		TS string
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || !out.OK {
		t.Fatalf("chat.postMessage = %+v, %v", out, err)
	}
	return out.TS
}

func TestStateIsTheFullFixture(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	want, err := app.Service.Export(t.Context())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	r := rawDo(t, app, http.MethodGet, "/_localslack/state", "")

	if r.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	if diff := cmp.Diff(want, r.fixture(t)); diff != "" {
		t.Errorf("state (-export +http):\n%s", diff)
	}
}

func TestSnapshotCreateListGet(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	want, _ := app.Service.Export(t.Context())

	created := rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"base"}`)

	if created.Status != http.StatusCreated || created.Header.Get("Location") != "/_localslack/snapshots/base" {
		t.Fatalf("create = %d Location=%q: %s", created.Status, created.Header.Get("Location"), created.Body)
	}
	var info struct {
		Name    string
		Created string
		Counts  map[string]int
	}
	if err := json.Unmarshal(created.Body, &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Name != "base" || info.Created == "" || info.Counts["users"] != len(want.Users) || info.Counts["messages"] != len(want.Messages) {
		t.Errorf("create reply = %+v", info)
	}

	list := rawDo(t, app, http.MethodGet, "/_localslack/snapshots", "")
	var page struct{ Items []struct{ Name string } }
	if err := json.Unmarshal(list.Body, &page); err != nil || list.Status != http.StatusOK || len(page.Items) != 1 || page.Items[0].Name != "base" {
		t.Errorf("list = %d %s (%v)", list.Status, list.Body, err)
	}

	got := rawDo(t, app, http.MethodGet, "/_localslack/snapshots/base", "")
	if got.Status != http.StatusOK {
		t.Fatalf("get = %d: %s", got.Status, got.Body)
	}
	if diff := cmp.Diff(want, got.fixture(t)); diff != "" {
		t.Errorf("snapshot fixture (-export +http):\n%s", diff)
	}
}

func TestSnapshotErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		verb, path string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"bad json", "POST", "/_localslack/snapshots", `{`, 400, "invalid_json"},
		{"unknown field", "POST", "/_localslack/snapshots", `{"name":"a","x":1}`, 400, "invalid_json"},
		{"bad name", "POST", "/_localslack/snapshots", `{"name":"a b"}`, 422, "invalid_snapshot_name"},
		{"reserved name", "POST", "/_localslack/snapshots", `{"name":"current"}`, 422, "invalid_snapshot_name"},
		{"missing name", "POST", "/_localslack/snapshots", `{}`, 422, "invalid_snapshot_name"},
		{"get unknown", "GET", "/_localslack/snapshots/ghost", "", 404, "not_found"},
		{"restore unknown", "POST", "/_localslack/snapshots/ghost/restore", "", 404, "not_found"},
		{"diff missing from", "GET", "/_localslack/diff", "", 400, "invalid_query"},
		{"diff unknown from", "GET", "/_localslack/diff?from=ghost", "", 404, "not_found"},
		{"diff unknown to", "GET", "/_localslack/diff?from=current&to=ghost", "", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)

			r := rawDo(t, app, tt.verb, tt.path, tt.body)

			if e := r.apiError(t); r.Status != tt.wantStatus || e.Error.Code != tt.wantCode || e.Error.Message == "" {
				t.Errorf("got %d %s, want %d %s with a message", r.Status, r.Body, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestSnapshotDuplicateAndLimit(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"dup"}`)

	r := rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"dup"}`)

	if e := r.apiError(t); r.Status != http.StatusConflict || e.Error.Code != "snapshot_exists" {
		t.Errorf("duplicate = %d %s, want 409 snapshot_exists", r.Status, r.Body)
	}
	for i := 1; i < control.MaxSnapshots; i++ {
		if r := rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"n`+strconv.Itoa(i)+`"}`); r.Status != http.StatusCreated {
			t.Fatalf("snapshot %d = %d %s", i, r.Status, r.Body)
		}
	}
	r = rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"overflow"}`)
	if e := r.apiError(t); r.Status != http.StatusConflict || e.Error.Code != "snapshot_limit_reached" {
		t.Errorf("limit = %d %s, want 409 snapshot_limit_reached", r.Status, r.Body)
	}
}

func TestRestoreReturnsToTheSnapshotAndKeepsTheCallLog(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	before, _ := app.Service.Export(t.Context())
	rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"base"}`)
	postGeneral(t, app, "after the snapshot")
	if mid, _ := app.Service.Export(t.Context()); len(mid.Messages) != len(before.Messages)+1 {
		t.Fatalf("setup: message not posted")
	}

	r := rawDo(t, app, http.MethodPost, "/_localslack/snapshots/base/restore", "")

	if r.Status != http.StatusOK {
		t.Fatalf("restore = %d: %s", r.Status, r.Body)
	}
	var res struct {
		Name   string
		Counts map[string]int
		TookMS *int64 `json:"took_ms"`
	}
	if err := json.Unmarshal(r.Body, &res); err != nil || res.Name != "base" || res.Counts["messages"] != len(before.Messages) || res.TookMS == nil {
		t.Errorf("restore reply = %s (%v)", r.Body, err)
	}
	after, _ := app.Service.Export(t.Context())
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("state after restore (-snapshot +got):\n%s", diff)
	}
	_, calls := listCalls(t, app.URL, "method=chat.postMessage")
	if len(calls.Items) != 1 {
		t.Errorf("call log entries for chat.postMessage = %d, want the pre-restore call kept", len(calls.Items))
	}
	if again := rawDo(t, app, http.MethodGet, "/_localslack/snapshots/base", ""); again.Status != http.StatusOK {
		t.Errorf("snapshot gone after restore: %d", again.Status)
	}
}

func TestDiffShowsActivitySinceSnapshot(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"base"}`)
	ts := postGeneral(t, app, "diff me")

	r := rawDo(t, app, http.MethodGet, "/_localslack/diff?from=base&to=current", "")

	if r.Status != http.StatusOK {
		t.Fatalf("diff = %d: %s", r.Status, r.Body)
	}
	var d scenario.Delta
	if err := json.Unmarshal(r.Body, &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(d.Messages.Added) != 1 || d.Messages.Added[0].TS != ts || d.Messages.Added[0].Text != "diff me" {
		t.Errorf("messages.added = %+v, want the posted message %s", d.Messages.Added, ts)
	}
	if len(d.Users.Added)+len(d.Users.Removed)+len(d.Conversations.Added)+len(d.Reactions.Added) != 0 {
		t.Errorf("unexpected other changes: %s", r.Body)
	}
	for _, key := range []string{`"users":`, `"conversations":`, `"members":`, `"messages":`, `"reactions":`, `"added":[]`, `"changed":[]`} {
		if !strings.Contains(string(r.Body), key) {
			t.Errorf("diff JSON lacks %s: %s", key, r.Body)
		}
	}
	// `to` defaults to current.
	if def := rawDo(t, app, http.MethodGet, "/_localslack/diff?from=base", ""); def.Status != http.StatusOK || string(def.Body) != string(r.Body) {
		t.Errorf("diff without to = %d %s, want same as to=current", def.Status, def.Body)
	}
}

func TestResetClearsSnapshots(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	rawDo(t, app, http.MethodPost, "/_localslack/snapshots", `{"name":"base"}`)

	rawDo(t, app, http.MethodPost, "/_localslack/reset", `{"seed":"fresh"}`)

	if r := rawDo(t, app, http.MethodGet, "/_localslack/snapshots/base", ""); r.Status != http.StatusNotFound {
		t.Errorf("snapshot after reset = %d, want 404", r.Status)
	}
}

const inlineScenario = `{
  "users":[{"id":"UANN","name":"ann"},{"id":"UBOT","name":"bot","is_bot":true,"bot_id":"BBOT"}],
  "channels":[{"id":"CGEN","name":"general","members":["UANN","UBOT"]}],
  "messages":[{"channel":"CGEN","ts":"1700000000.000001","user":"UANN","text":"hello"}]
}`

func TestResetWithInlineScenario(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)

	r := rawDo(t, app, http.MethodPost, "/_localslack/reset", `{"scenario":`+inlineScenario+`}`)

	if r.Status != http.StatusOK {
		t.Fatalf("reset = %d: %s", r.Status, r.Body)
	}
	var got resetReply
	if err := json.Unmarshal(r.Body, &got); err != nil || got.Counts.Users != 2 || got.Counts.Channels != 1 || got.Counts.Messages != 1 {
		t.Errorf("reply = %+v (%v), want 2 users, 1 channel, 1 message", got, err)
	}
	st := rawDo(t, app, http.MethodGet, "/_localslack/state", "").fixture(t)
	if len(st.Users) != 2 || st.Users[0].ID != "UANN" || st.Messages[0].TS != "1700000000.000001" {
		t.Errorf("state = %+v, want the scenario's exact ids", st)
	}
}

func TestResetWithScenarioFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "world.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
users: [{id: UANN, name: ann}]
channels: [{id: CGEN, name: general, members: [UANN]}]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"users":[{"id":"X1","name":"ann"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantCode   string
		wantPtr    string
	}{
		{"yaml file", yamlPath, 200, "", ""},
		{"invalid content", badPath, 422, "invalid_scenario", "/scenario/users/0/id"},
		{"missing file", filepath.Join(dir, "nope.json"), 400, "invalid_reset", ""},
		{"wrong extension", filepath.Join(dir, "x.txt"), 400, "invalid_reset", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)

			r := rawDo(t, app, http.MethodPost, "/_localslack/reset", `{"scenario":`+strconv.Quote(tt.path)+`}`)

			if r.Status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", r.Status, tt.wantStatus, r.Body)
			}
			if tt.wantCode == "" {
				return
			}
			e := r.apiError(t)
			if e.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", e.Error.Code, tt.wantCode)
			}
			if tt.wantPtr != "" && (len(e.Error.Details) == 0 || e.Error.Details[0].Pointer != tt.wantPtr) {
				t.Errorf("details = %+v, want first pointer %s", e.Error.Details, tt.wantPtr)
			}
			if strings.Contains(string(r.Body), "ann") {
				t.Errorf("error echoes file content: %s", r.Body)
			}
		})
	}
}

func TestResetScenarioAndGenerateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		wantPtr    string
	}{
		{"computed field", `{"scenario":{"messages":[{"channel":"C","reply_count":1}]}}`, 422, "invalid_scenario", "/scenario/messages/0/reply_count"},
		{"unknown scenario field", `{"scenario":{"usrs":[]}}`, 422, "invalid_scenario", "/scenario/usrs"},
		{"semantic problems", `{"scenario":{"users":[{"id":"U1","name":"a"},{"id":"U1","name":"b"}]}}`, 422, "invalid_scenario", "/scenario/users/1/id"},
		{"scenario wrong type", `{"scenario":5}`, 400, "invalid_reset", ""},
		{"generate bad param", `{"generate":{"users":99999}}`, 422, "invalid_generate", "/generate/users"},
		{"generate unknown field", `{"generate":{"people":3}}`, 400, "invalid_reset", ""},
		{"generate mixed into scenario", `{"scenario":{"generate":{"users":99999}}}`, 422, "invalid_scenario", "/scenario/generate/users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := storetest.NewApp(t)
			before := mustUsers(t, app)

			r := rawDo(t, app, http.MethodPost, "/_localslack/reset", tt.body)

			e := r.apiError(t)
			if r.Status != tt.wantStatus || e.Error.Code != tt.wantCode {
				t.Fatalf("got %d %s, want %d %s", r.Status, r.Body, tt.wantStatus, tt.wantCode)
			}
			if tt.wantPtr != "" && !hasPointer(e.Error.Details, tt.wantPtr) {
				t.Errorf("details = %+v, want pointer %s", e.Error.Details, tt.wantPtr)
			}
			if diff := cmp.Diff(before, mustUsers(t, app)); diff != "" {
				t.Errorf("rejected reset changed users (-before +after):\n%s", diff)
			}
		})
	}
}

func hasPointer(issues []scenario.Issue, ptr string) bool {
	for _, i := range issues {
		if i.Pointer == ptr {
			return true
		}
	}
	return false
}

func TestResetWithGenerate(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)

	r := rawDo(t, app, http.MethodPost, "/_localslack/reset", `{"seed":"gen","generate":{"users":8,"channels":3,"messages_per_channel":5,"thread_ratio":0.4,"private_channels":1}}`)

	var got resetReply
	if err := json.Unmarshal(r.Body, &got); err != nil || r.Status != http.StatusOK {
		t.Fatalf("reset = %d %s (%v)", r.Status, r.Body, err)
	}
	if got.Counts.Users != 9 || got.Counts.Channels != 3 || got.Counts.Messages != 15 || got.Seed != "gen" {
		t.Errorf("reply = %+v, want 9 users, 3 channels, 15 messages, seed gen", got)
	}
}
