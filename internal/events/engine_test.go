package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/events"
	"localslack/internal/service"
)

const waitFor = 10 * time.Second

type fakeSource struct {
	mu       sync.Mutex
	apps     []service.App
	verified map[string]bool
	gate     chan struct{} // when set, Apps() blocks until closed
}

func (f *fakeSource) Apps() []service.App {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]service.App(nil), f.apps...)
}

func (f *fakeSource) Team(context.Context) (service.Team, error) { return service.Team{ID: "T1"}, nil }

func (f *fakeSource) SetAppVerified(_ context.Context, id string, v bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.verified == nil {
		f.verified = map[string]bool{}
	}
	f.verified[id] = v
	return nil
}

func (f *fakeSource) isVerified(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verified[id]
}

// fakeApp is an httptest Slack app: it verifies signatures with its own HMAC and
// answers with scripted statuses (the last one repeats).
type fakeApp struct {
	*httptest.Server
	secret string

	mu       sync.Mutex
	reqs     []seen
	sigOK    []bool
	statuses []int
	headers  map[string]string
	respond  func(http.ResponseWriter, *http.Request)
}

func newFakeApp(t *testing.T, secret string, statuses ...int) *fakeApp {
	t.Helper()
	if len(statuses) == 0 {
		statuses = []int{200}
	}
	a := &fakeApp{secret: secret, statuses: statuses}
	a.Server = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.Close)
	return a
}

func (a *fakeApp) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body)) // scripted responders may read it again
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	ok := r.Header.Get("X-Slack-Signature") == events.Sign(a.secret, ts, body)
	a.mu.Lock()
	n := len(a.reqs)
	a.reqs = append(a.reqs, seen{Method: r.Method, Path: r.URL.Path, Body: string(body), Header: r.Header.Clone()})
	a.sigOK = append(a.sigOK, ok)
	status := a.statuses[min(n, len(a.statuses)-1)]
	respond := a.respond
	for k, v := range a.headers {
		w.Header().Set(k, v)
	}
	a.mu.Unlock()
	if respond != nil {
		respond(w, r)
		return
	}
	w.WriteHeader(status)
}

func (a *fakeApp) requests() []seen {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]seen(nil), a.reqs...)
}

func (a *fakeApp) signatureOK() []bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]bool(nil), a.sigOK...)
}

func (a *fakeApp) appDef(id, botUser string, subs ...string) service.App {
	return service.App{
		ID: id, BotUserID: botUser, BotID: "B" + id, SigningSecret: a.secret, VerificationToken: "vt-" + id,
		BotEvents: subs, DeliverTo: a.URL,
	}
}

func fastConfig() events.DeliveryConfig {
	return events.DeliveryConfig{Signature: events.SigValid, StaleSeconds: 600, TimeoutMS: 2000, RetryDelaysMS: []int64{0, 10, 20}}
}

type engineOpts struct {
	cfg    events.DeliveryConfig
	clock  events.Clock
	queue  int
	policy events.URLPolicy
}

func newEngine(t *testing.T, src *fakeSource, o engineOpts) *events.Engine {
	t.Helper()
	if o.cfg.TimeoutMS == 0 {
		o.cfg = fastConfig()
	}
	var clock events.Clock = events.SystemClock{}
	if o.clock != nil {
		clock = o.clock
	}
	e := events.New(events.Options{
		Source: src, Clock: clock, Transport: events.NewHTTPTransport(clock), Policy: o.policy, Config: o.cfg,
		TraceSize: 100, QueueSize: o.queue, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func posted(text string, targets ...string) service.MessagePosted {
	return service.MessagePosted{
		Msg:   service.Message{ChannelID: "C1", TS: "100.000001", UserID: "UHUMAN", Text: text},
		Scope: service.Scope{Channel: service.ChannelInfo{ID: "C1", Name: "general", Kind: service.KindChannel}, Targets: targets},
	}
}

func waitState(t *testing.T, e *events.Engine, state string, n int) []events.Delivery {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), waitFor+time.Second)
	defer cancel()
	got, _, _ := e.WaitDeliveries(ctx, events.Filter{State: state}, n, waitFor)
	if len(got) < n {
		all, _, _ := e.Deliveries(events.Filter{})
		t.Fatalf("wanted %d %s deliveries, got %d; all: %+v", n, state, len(got), all)
	}
	return got
}

func TestEngineDeliversSignedEnvelope(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateDelivered, 1)

	reqs := app.requests()
	if len(reqs) != 1 {
		t.Fatalf("app got %d requests, want 1", len(reqs))
	}
	if ok := app.signatureOK(); !ok[0] {
		t.Error("signature did not verify with the app's own HMAC")
	}
	if reqs[0].Header.Get("X-Slack-Retry-Num") != "" {
		t.Error("first attempt must not carry X-Slack-Retry-Num")
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(reqs[0].Body), &env); err != nil {
		t.Fatalf("body: %v", err)
	}
	if env["type"] != "event_callback" || env["api_app_id"] != "A1" || env["team_id"] != "T1" || env["token"] != "vt-A1" {
		t.Errorf("envelope = %v", env)
	}
	evID, _ := env["event_id"].(string)
	if !strings.HasPrefix(evID, "Ev") || got[0].EventID != evID {
		t.Errorf("event_id = %q, delivery says %q", evID, got[0].EventID)
	}
	if got[0].EventType != "message" || got[0].AppID != "A1" || got[0].Transport != "http" || got[0].Injected {
		t.Errorf("delivery = %+v", got[0])
	}
	if len(got[0].Attempts) != 1 || got[0].Attempts[0].Status != 200 || got[0].Attempts[0].N != 1 {
		t.Errorf("attempts = %+v", got[0].Attempts)
	}
}

func TestEngineRetrySequence(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek", 500, 503, 200)
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateDelivered, 1)

	reqs := app.requests()
	type hdr struct{ Num, Reason string }
	var gotHdrs []hdr
	for _, r := range reqs {
		gotHdrs = append(gotHdrs, hdr{r.Header.Get("X-Slack-Retry-Num"), r.Header.Get("X-Slack-Retry-Reason")})
	}
	want := []hdr{{"", ""}, {"1", "http_error"}, {"2", "http_error"}}
	if diff := cmp.Diff(want, gotHdrs); diff != "" {
		t.Errorf("retry headers (-want +got):\n%s", diff)
	}
	var statuses []int
	for _, a := range got[0].Attempts {
		statuses = append(statuses, a.Status)
	}
	if diff := cmp.Diff([]int{500, 503, 200}, statuses); diff != "" {
		t.Errorf("attempt statuses (-want +got):\n%s", diff)
	}
}

func TestEngineGivesUpAfterConfiguredRetries(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek", 500)
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateFailed, 1)

	if n := len(got[0].Attempts); n != 4 {
		t.Errorf("attempts = %d, want 1 + 3 retries", n)
	}
	if n := len(app.requests()); n != 4 {
		t.Errorf("app requests = %d, want 4", n)
	}
}

func TestEngineNoRetryHeaderStopsRetries(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek", 500)
	app.headers = map[string]string{"X-Slack-No-Retry": "1"}
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateFailed, 1)
	if n := len(got[0].Attempts); n != 1 {
		t.Errorf("attempts = %d, want 1 (no-retry)", n)
	}
}

func TestEngineTimeoutThenRetryReason(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek")
	var calls atomic.Int32
	app.respond = func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusOK)
	}
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	cfg := fastConfig()
	cfg.TimeoutMS = 50
	e := newEngine(t, src, engineOpts{cfg: cfg})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateDelivered, 1)

	first, second := got[0].Attempts[0], got[0].Attempts[1]
	if first.Error != "timeout" || first.Status != 0 {
		t.Errorf("first attempt = %+v, want timeout", first)
	}
	if second.RetryReason != "http_timeout" {
		t.Errorf("second attempt retry_reason = %q, want http_timeout", second.RetryReason)
	}
	if h := app.requests()[1].Header.Get("X-Slack-Retry-Reason"); h != "http_timeout" {
		t.Errorf("X-Slack-Retry-Reason = %q, want http_timeout", h)
	}
}

func TestEngineRetryWaitUsesInjectedClock(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "sek", 500, 200)
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	clock := events.NewManualClock(time.Unix(1_700_000_000, 0))
	cfg := fastConfig()
	cfg.RetryDelaysMS = []int64{60_000}
	e := newEngine(t, src, engineOpts{cfg: cfg, clock: clock})

	e.Publish(posted("hello", "UBOT1"))
	waitState(t, e, events.StateRetrying, 1)
	for clock.Waiting() == 0 {
		time.Sleep(time.Millisecond) // the retry timer registers just after the state flips
	}
	if n := len(app.requests()); n != 1 {
		t.Fatalf("requests before the clock moved = %d, want 1", n)
	}
	clock.Advance(time.Minute)
	waitState(t, e, events.StateDelivered, 1)
	if n := len(app.requests()); n != 2 {
		t.Errorf("requests after retry = %d, want 2", n)
	}
}

func TestEngineTwoAppsGetDistinctEnvelopes(t *testing.T) {
	t.Parallel()

	a := newFakeApp(t, "secret-a")
	b := newFakeApp(t, "secret-b")
	src := &fakeSource{apps: []service.App{
		a.appDef("A1", "UBOT1", "message.channels"),
		b.appDef("A2", "UBOT2", "message.channels"),
	}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hi both", "UBOT1", "UBOT2"))
	waitState(t, e, events.StateDelivered, 2)

	env := func(app *fakeApp) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(app.requests()[0].Body), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m
	}
	ea, eb := env(a), env(b)
	if ea["api_app_id"] != "A1" || eb["api_app_id"] != "A2" {
		t.Errorf("api_app_id = %v / %v", ea["api_app_id"], eb["api_app_id"])
	}
	if ea["event_id"] == eb["event_id"] {
		t.Errorf("both apps got event_id %v, want distinct", ea["event_id"])
	}
	authz := func(m map[string]any) string {
		return m["authorizations"].([]any)[0].(map[string]any)["user_id"].(string)
	}
	if authz(ea) != "UBOT1" || authz(eb) != "UBOT2" {
		t.Errorf("authorizations = %v / %v", authz(ea), authz(eb))
	}
	if !a.signatureOK()[0] || !b.signatureOK()[0] {
		t.Error("each app's signature must verify with its own secret")
	}
}

func TestEngineOnlyTargetedAppsGetMessages(t *testing.T) {
	t.Parallel()

	a := newFakeApp(t, "s")
	b := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{
		a.appDef("A1", "UBOT1", "message.channels"),
		b.appDef("A2", "UBOT2", "message.channels"),
	}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("only A1 is in the channel", "UBOT1"))
	waitState(t, e, events.StateDelivered, 1)
	if n := len(b.requests()); n != 0 {
		t.Errorf("non-member app got %d requests, want 0", n)
	}
}

func TestEngineBlockedRemoteURLIsRecordedNotSent(t *testing.T) {
	t.Parallel()

	src := &fakeSource{apps: []service.App{{
		ID: "A1", BotUserID: "UBOT1", SigningSecret: "s", BotEvents: []string{"message.channels"},
		RequestURL: "https://example.invalid/slack/events",
	}}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	got := waitState(t, e, events.StateFailed, 1)
	if len(got[0].Attempts) != 1 || !strings.Contains(got[0].Attempts[0].Error, "delivery blocked") {
		t.Errorf("attempts = %+v, want one 'delivery blocked' attempt", got[0].Attempts)
	}
	if tgt := e.Resolve(src.apps[0]); !tgt.Blocked {
		t.Errorf("Resolve = %+v, want blocked", tgt)
	}
}

func TestEngineDeliverToHostOnlyKeepsManifestPath(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	def := app.appDef("A1", "UBOT1", "message.channels")
	def.RequestURL = "https://example.invalid/hooks/slack"
	src := &fakeSource{apps: []service.App{def}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("hello", "UBOT1"))
	waitState(t, e, events.StateDelivered, 1)
	if p := app.requests()[0].Path; p != "/hooks/slack" {
		t.Errorf("path = %q, want the manifest path /hooks/slack", p)
	}
}

func TestEngineResetDropsLateResults(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	release := make(chan struct{})
	inflight := make(chan struct{}, 1)
	app.respond = func(w http.ResponseWriter, _ *http.Request) {
		select {
		case inflight <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("slow", "UBOT1"))
	<-inflight
	e.Pause()
	e.Reset()
	e.Resume()
	close(release)
	if err := e.Close(); err != nil { // waits for the in-flight attempt to finish
		t.Fatalf("Close: %v", err)
	}
	if got, _, _ := e.Deliveries(events.Filter{}); len(got) != 0 {
		t.Errorf("deliveries after reset = %+v, want none (late result must not be recorded)", got)
	}
}

func TestEngineSeqSurvivesDeleteAndReset(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("one", "UBOT1"))
	first := waitState(t, e, events.StateDelivered, 1)
	e.ClearDeliveries()
	e.Publish(posted("two", "UBOT1"))
	second := waitState(t, e, events.StateDelivered, 1)
	e.Reset()
	e.Publish(posted("three", "UBOT1"))
	third := waitState(t, e, events.StateDelivered, 1)
	if first[0].Seq >= second[0].Seq || second[0].Seq >= third[0].Seq {
		t.Errorf("seq = %d, %d, %d, want strictly increasing", first[0].Seq, second[0].Seq, third[0].Seq)
	}
}

func TestEnginePausedConfigHoldsThenFlushes(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	paused := true
	if _, err := e.SetConfig(events.ConfigPatch{Paused: &paused}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	e.Publish(posted("held", "UBOT1"))
	waitState(t, e, events.StatePending, 1)
	if n := len(app.requests()); n != 0 {
		t.Fatalf("paused engine delivered %d requests", n)
	}

	paused = false
	if _, err := e.SetConfig(events.ConfigPatch{Paused: &paused}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	waitState(t, e, events.StateDelivered, 1)
}

func TestEnginePauseResumeDropsPublishes(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Pause()
	e.Publish(posted("during reset", "UBOT1"))
	e.Resume()
	e.Publish(posted("after", "UBOT1"))
	got := waitState(t, e, events.StateDelivered, 1)

	var env struct{ Event struct{ Text string } }
	if err := json.Unmarshal(got[0].Payload, &env); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if env.Event.Text != "after" {
		t.Errorf("delivered text = %q, want only the post-Resume event", env.Event.Text)
	}
}

func TestEngineQueueOverflowCountsDrops(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	src := &fakeSource{gate: gate}
	e := newEngine(t, src, engineOpts{queue: 2})

	for range 20 {
		e.Publish(posted("x", "UBOT1"))
	}
	if e.Dropped() == 0 {
		t.Error("Dropped = 0, want overflow counted")
	}
	close(gate)
}

func TestEngineVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		respond      func(http.ResponseWriter, *http.Request)
		wantVerified bool
	}{
		{
			name: "json challenge echo",
			respond: func(w http.ResponseWriter, r *http.Request) {
				var b struct{ Challenge string }
				_ = json.NewDecoder(r.Body).Decode(&b)
				_ = json.NewEncoder(w).Encode(map[string]string{"challenge": b.Challenge})
			},
			wantVerified: true,
		},
		{
			name: "plain text echo",
			respond: func(w http.ResponseWriter, r *http.Request) {
				var b struct{ Challenge string }
				_ = json.NewDecoder(r.Body).Decode(&b)
				_, _ = io.WriteString(w, b.Challenge)
			},
			wantVerified: true,
		},
		{name: "wrong answer", respond: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "nope") }},
		{name: "server error", respond: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newFakeApp(t, "sek")
			app.respond = tt.respond
			src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
			e := newEngine(t, src, engineOpts{})

			res, err := e.Verify(t.Context(), "A1")
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if res.Verified != tt.wantVerified {
				t.Errorf("Verified = %v (%s), want %v", res.Verified, res.Error, tt.wantVerified)
			}
			if src.isVerified("A1") != tt.wantVerified {
				t.Errorf("source verified flag = %v, want %v", src.isVerified("A1"), tt.wantVerified)
			}
			if !app.signatureOK()[0] {
				t.Error("url_verification must be signed")
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(app.requests()[0].Body), &body); err != nil {
				t.Fatalf("body: %v", err)
			}
			if body["type"] != "url_verification" || body["token"] != "vt-A1" || body["challenge"] == "" {
				t.Errorf("verification body = %v", body)
			}
			if (e.VerifyError("A1") == "") != tt.wantVerified {
				t.Errorf("VerifyError = %q", e.VerifyError("A1"))
			}
		})
	}
}

func TestEngineVerifyUnknownApp(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	if _, err := e.Verify(t.Context(), "A404"); !errors.Is(err, events.ErrAppNotFound) {
		t.Errorf("Verify err = %v, want ErrAppNotFound", err)
	}
}

func TestEngineInjectWithBadSignature(t *testing.T) {
	t.Parallel()

	a := newFakeApp(t, "s")
	b := newFakeApp(t, "s")
	c := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{
		a.appDef("A1", "UBOT1", "reaction_added"),
		b.appDef("A2", "UBOT2", "reaction_added", "message.channels"),
		c.appDef("A3", "UBOT3", "app_mention"),
	}}
	e := newEngine(t, src, engineOpts{})

	res, err := e.Inject(t.Context(), events.InjectRequest{
		Event:     map[string]any{"type": "reaction_added", "reaction": "tada"},
		Signature: events.SigInvalid,
	})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if len(res.Deliveries) != 2 || res.Deliveries[0].EventID == "" || res.Deliveries[0].EventID == res.Deliveries[1].EventID {
		t.Fatalf("result = %+v, want one delivery per subscribed app, each with its own event id", res)
	}
	got := waitState(t, e, events.StateDelivered, 2)
	wantIDs := map[string]string{} // delivery id -> event id
	for _, ref := range res.Deliveries {
		wantIDs[ref.DeliveryID] = ref.EventID
	}
	for _, d := range got {
		if wantIDs[d.ID] != d.EventID || !d.Injected {
			t.Errorf("delivery %+v, want injected with event id %q", d, wantIDs[d.ID])
		}
	}
	for name, app := range map[string]*fakeApp{"A1": a, "A2": b} {
		if ok := app.signatureOK(); len(ok) != 1 || ok[0] {
			t.Errorf("%s: signature verified = %v, want one request with an invalid signature", name, ok)
		}
	}
	if n := len(c.requests()); n != 0 {
		t.Errorf("unsubscribed app got %d requests", n)
	}
}

func TestEngineInjectTargetsAppAndMergesEnvelope(t *testing.T) {
	t.Parallel()

	a := newFakeApp(t, "s")
	b := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{a.appDef("A1", "UBOT1"), b.appDef("A2", "UBOT2")}}
	e := newEngine(t, src, engineOpts{})

	res, err := e.Inject(t.Context(), events.InjectRequest{
		Event: map[string]any{"type": "team_join"}, AppID: "A2", Envelope: map[string]any{"team_id": "TCUSTOM"},
	})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if len(res.Deliveries) != 1 || res.Deliveries[0].AppID != "A2" {
		t.Fatalf("deliveries = %+v, want only A2 (explicit app ignores subscriptions)", res.Deliveries)
	}
	got := waitState(t, e, events.StateDelivered, 1)
	if !got[0].Injected {
		t.Error("delivery.injected = false")
	}
	if len(a.requests()) != 0 {
		t.Error("untargeted app received the injected event")
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(b.requests()[0].Body), &env); err != nil {
		t.Fatalf("body: %v", err)
	}
	if env["team_id"] != "TCUSTOM" || env["event_id"] != res.Deliveries[0].EventID {
		t.Errorf("envelope = %v", env)
	}
}

func TestEngineInjectValidation(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	if _, err := e.Inject(t.Context(), events.InjectRequest{Event: map[string]any{"x": 1}}); !errors.Is(err, events.ErrInvalidEvent) {
		t.Errorf("no type: err = %v, want ErrInvalidEvent", err)
	}
	if _, err := e.Inject(t.Context(), events.InjectRequest{Event: map[string]any{"type": "x"}, AppID: "nope"}); !errors.Is(err, events.ErrAppNotFound) {
		t.Errorf("unknown app: err = %v, want ErrAppNotFound", err)
	}
	if _, err := e.Inject(t.Context(), events.InjectRequest{Event: map[string]any{"type": "x"}, Signature: "bogus"}); !errors.Is(err, events.ErrInvalidEvent) {
		t.Errorf("bad signature mode: err = %v, want ErrInvalidEvent", err)
	}
}

func TestEngineRedeliverSendsDuplicateWithRetryHeaders(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})

	e.Publish(posted("dup me", "UBOT1"))
	first := waitState(t, e, events.StateDelivered, 1)

	res, err := e.Redeliver(t.Context(), first[0].EventID, events.RedeliverRequest{RetryNum: 1, RetryReason: "http_timeout"})
	if err != nil {
		t.Fatalf("Redeliver: %v", err)
	}
	if want := []events.DeliveryRef{{AppID: "A1", EventID: first[0].EventID, DeliveryID: res.Deliveries[0].DeliveryID}}; !cmp.Equal(res.Deliveries, want) || res.Deliveries[0].DeliveryID == first[0].ID {
		t.Fatalf("result = %+v, want a new delivery of event %s to A1", res, first[0].EventID)
	}
	waitState(t, e, events.StateDelivered, 2)

	reqs := app.requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	if reqs[0].Body != reqs[1].Body {
		t.Error("redelivered body differs from the original")
	}
	if reqs[1].Header.Get("X-Slack-Retry-Num") != "1" || reqs[1].Header.Get("X-Slack-Retry-Reason") != "http_timeout" {
		t.Errorf("redelivery headers = %v", reqs[1].Header)
	}
}

func TestEngineRedeliverUnknownEvent(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	if _, err := e.Redeliver(t.Context(), "Ev404", events.RedeliverRequest{}); !errors.Is(err, events.ErrEventNotFound) {
		t.Errorf("err = %v, want ErrEventNotFound", err)
	}
}

func TestEnginePerAppConfigOverridesGlobal(t *testing.T) {
	t.Parallel()

	a := newFakeApp(t, "s")
	b := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{
		a.appDef("A1", "UBOT1", "message.channels"),
		b.appDef("A2", "UBOT2", "message.channels"),
	}}
	e := newEngine(t, src, engineOpts{})

	bad := events.SigInvalid
	if _, err := e.SetAppConfig("A2", events.ConfigPatch{Signature: &bad}); err != nil {
		t.Fatalf("SetAppConfig: %v", err)
	}
	e.Publish(posted("hi", "UBOT1", "UBOT2"))
	waitState(t, e, events.StateDelivered, 2)

	if !a.signatureOK()[0] {
		t.Error("app A1 should get valid signatures")
	}
	if b.signatureOK()[0] {
		t.Error("app A2 override requested invalid signatures")
	}
	if cfg, override := e.AppConfig("A2"); !cmp.Equal(override, events.ConfigPatch{Signature: &bad}) || cfg.Signature != events.SigInvalid {
		t.Errorf("AppConfig(A2) = %+v, override=%+v", cfg, override)
	}
	if cfg, override := e.AppConfig("A1"); !override.IsZero() || cfg.Signature != events.SigValid {
		t.Errorf("AppConfig(A1) = %+v, override=%+v", cfg, override)
	}
	e.ClearAppConfig("A2")
	if _, override := e.AppConfig("A2"); !override.IsZero() {
		t.Error("override survived ClearAppConfig")
	}
}

func TestEngineResetRestoresDefaultConfig(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	bad := events.SigInvalid
	if _, err := e.SetConfig(events.ConfigPatch{Signature: &bad}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	e.Reset()
	if got := e.Config().Signature; got != events.SigValid {
		t.Errorf("signature after reset = %q, want default", got)
	}
}

func TestEngineConfigPatchRejectsInvalid(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	zero := int64(0)
	if _, err := e.SetConfig(events.ConfigPatch{TimeoutMS: &zero}); err == nil {
		t.Error("SetConfig accepted timeout 0")
	}
	if got := e.Config().TimeoutMS; got != 2000 {
		t.Errorf("timeout changed to %d by a rejected patch", got)
	}
}

func TestEngineIDsLookLikeSlackIDs(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{app.appDef("A1", "UBOT1", "message.channels")}}
	e := newEngine(t, src, engineOpts{})
	for i := range 3 {
		e.Publish(posted("m"+strconv.Itoa(i), "UBOT1"))
	}
	got := waitState(t, e, events.StateDelivered, 3)
	seen := map[string]bool{}
	for _, d := range got {
		if len(d.EventID) != 12 || !strings.HasPrefix(d.EventID, "Ev") || seen[d.EventID] {
			t.Errorf("event id %q: want unique Ev + 10 chars", d.EventID)
		}
		seen[d.EventID] = true
	}
}

func TestEngineRedeliverByDeliveryAndEventID(t *testing.T) {
	t.Parallel()

	app := newFakeApp(t, "s")
	other := newFakeApp(t, "s")
	src := &fakeSource{apps: []service.App{
		app.appDef("A1", "UBOT1", "message.channels"), other.appDef("A2", "UBOT2", "message.channels"),
	}}
	e := newEngine(t, src, engineOpts{})
	e.Publish(posted("dup me", "UBOT1", "UBOT2"))
	first := waitState(t, e, events.StateDelivered, 2)
	byApp := map[string]events.Delivery{first[0].AppID: first[0], first[1].AppID: first[1]}
	if byApp["A1"].EventID == byApp["A2"].EventID {
		t.Fatalf("both apps share event id %q; event ids are per app delivery", byApp["A1"].EventID)
	}

	total := 2
	tests := []struct {
		name    string
		redeliv func() (events.InjectResult, error)
		wantApp string
		wantErr error
	}{
		{"by event id", func() (events.InjectResult, error) {
			return e.Redeliver(t.Context(), byApp["A1"].EventID, events.RedeliverRequest{})
		}, "A1", nil},
		{"by event id and matching app", func() (events.InjectResult, error) {
			return e.Redeliver(t.Context(), byApp["A2"].EventID, events.RedeliverRequest{AppID: "A2"})
		}, "A2", nil},
		{"by event id and another app", func() (events.InjectResult, error) {
			return e.Redeliver(t.Context(), byApp["A1"].EventID, events.RedeliverRequest{AppID: "A2"})
		}, "", events.ErrEventNotFound},
		{"by delivery id", func() (events.InjectResult, error) {
			return e.RedeliverDelivery(t.Context(), byApp["A2"].ID, events.RedeliverRequest{})
		}, "A2", nil},
		{"unknown delivery id", func() (events.InjectResult, error) {
			return e.RedeliverDelivery(t.Context(), "D404", events.RedeliverRequest{})
		}, "", events.ErrDeliveryNotFound},
	}
	for _, tt := range tests { //nolint:paralleltest // ordered cases share one engine
		t.Run(tt.name, func(t *testing.T) {
			// Subtests share one engine and run in order, each adding at most one delivery.
			res, err := tt.redeliv()

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if len(res.Deliveries) != 1 || res.Deliveries[0].AppID != tt.wantApp || res.Deliveries[0].EventID != byApp[tt.wantApp].EventID {
				t.Fatalf("result = %+v, want one redelivery of %s's event %s", res, tt.wantApp, byApp[tt.wantApp].EventID)
			}
			total++
			waitState(t, e, events.StateDelivered, total)
		})
	}
}

func TestEngineReplaceConfigRestoresDefaultsForOmittedFields(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	bad, stale, timeout := events.SigInvalid, 30, int64(9000)
	if _, err := e.SetConfig(events.ConfigPatch{Signature: &bad, StaleSeconds: &stale}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	tests := []struct {
		name  string
		apply func(events.ConfigPatch) (events.DeliveryConfig, error)
		want  events.DeliveryConfig
	}{
		{"patch merges", e.SetConfig, withTimeout(withStale(withSig(fastConfig(), events.SigInvalid), 30), 9000)},
		{"replace resets omitted", e.ReplaceConfig, withTimeout(fastConfig(), 9000)},
	}
	for _, tt := range tests { //nolint:paralleltest // ordered cases share one engine
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.apply(events.ConfigPatch{TimeoutMS: &timeout})
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("config (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(got, e.Config()); diff != "" {
				t.Errorf("Config() disagrees with the reply (-reply +config):\n%s", diff)
			}
			// Restore the non-default start state for the next case.
			if _, err := e.ReplaceConfig(events.ConfigPatch{Signature: &bad, StaleSeconds: &stale}); err != nil {
				t.Fatalf("restore: %v", err)
			}
		})
	}
}

func withTimeout(c events.DeliveryConfig, ms int64) events.DeliveryConfig { c.TimeoutMS = ms; return c }

func withStale(c events.DeliveryConfig, s int) events.DeliveryConfig { c.StaleSeconds = s; return c }

func TestEnginePerAppOverridesAreSparseAndInheritLive(t *testing.T) {
	t.Parallel()

	e := newEngine(t, &fakeSource{}, engineOpts{})
	bad, paused, timeout := events.SigInvalid, true, int64(7000)
	if _, err := e.SetAppConfig("A1", events.ConfigPatch{Signature: &bad}); err != nil {
		t.Fatalf("SetAppConfig: %v", err)
	}
	if _, err := e.SetConfig(events.ConfigPatch{TimeoutMS: &timeout}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	eff, override := e.AppConfig("A1")
	if want := withTimeout(withSig(fastConfig(), events.SigInvalid), 7000); !cmp.Equal(eff, want) {
		t.Errorf("effective = %+v, want global change inherited live: %+v", eff, want)
	}
	if want := (events.ConfigPatch{Signature: &bad}); !cmp.Equal(override, want) {
		t.Errorf("override = %+v, want only the signature", override)
	}

	// PATCH-style merge adds a field; PUT-style replace drops the others.
	if _, err := e.SetAppConfig("A1", events.ConfigPatch{Paused: &paused}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, o := e.AppConfig("A1"); !cmp.Equal(o, events.ConfigPatch{Signature: &bad, Paused: &paused}) {
		t.Errorf("after merge override = %+v, want signature+paused", o)
	}
	if _, err := e.ReplaceAppConfig("A1", events.ConfigPatch{TimeoutMS: &timeout}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if eff, o := e.AppConfig("A1"); !cmp.Equal(o, events.ConfigPatch{TimeoutMS: &timeout}) || eff.Signature != events.SigValid || eff.Paused {
		t.Errorf("after replace effective=%+v override=%+v, want only timeout overridden", eff, o)
	}

	e.Reset()
	if _, o := e.AppConfig("A1"); !o.IsZero() {
		t.Errorf("reset kept per-app override %+v", o)
	}
}
