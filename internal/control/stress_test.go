package control_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"localslack/internal/appmanifest"
	"localslack/internal/control"
	"localslack/internal/events"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

// epochDispatcher wraps a real Engine and remembers, per Reset, which events
// were published before it. Anything delivered afterwards for those is a bug.
type epochDispatcher struct {
	*events.Engine
	tr *epochTransport
}

func (d *epochDispatcher) Publish(ev service.Event) {
	if p, ok := ev.(service.MessagePosted); ok {
		d.tr.published(p.Msg.TS)
	}
	d.Engine.Publish(ev)
}

// Reset runs the engine's reset, then declares everything published so far dead.
func (d *epochDispatcher) Reset() {
	d.Engine.Reset()
	d.tr.killPublished()
}

// epochTransport always fails (forcing retries) and flags attempts for dead events.
type epochTransport struct {
	mu         sync.Mutex
	pub, dead  map[string]bool
	violations []string
	attempts   int
}

func (t *epochTransport) published(ts string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pub[ts] = true
}

func (t *epochTransport) killPublished() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ts := range t.pub {
		t.dead[ts] = true
	}
}

func (t *epochTransport) Deliver(_ context.Context, d events.Delivery) events.Attempt {
	var p struct {
		Event struct{ TS string } `json:"event"`
	}
	_ = json.Unmarshal(d.Payload, &p)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attempts++
	if t.dead[p.Event.TS] {
		t.violations = append(t.violations, p.Event.TS)
	}
	return events.Attempt{Status: 500}
}

func TestNoPreResetEventIsDeliveredAfterReset(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	ctx := t.Context()
	tr := &epochTransport{pub: map[string]bool{}, dead: map[string]bool{}}
	eng := events.New(events.Options{
		Source: app.Service, Transport: tr,
		Config: events.DefaultDeliveryConfig(time.Second, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}),
	})
	disp := &epochDispatcher{Engine: eng, tr: tr}
	app.Service.Subscribe(disp.Publish)
	t.Cleanup(func() { _ = eng.Close() })
	manifest, err := appmanifest.Parse([]byte(`{"display_information":{"name":"Stress"},"settings":{"event_subscriptions":{"request_url":"http://127.0.0.1:9/ev","bot_events":["message.channels"]}}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := app.Service.RegisterApp(ctx, service.RegisterInput{Manifest: manifest}); err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	caller := firstHumanUser(t, app)
	general := generalChannel(t, app, caller)
	c := control.New(app.Service, disp, control.WithSeed(storetest.PinnedSeed))

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 60 {
				_, err := app.Service.PostMessage(ctx, caller, service.PostOpts{Channel: general, Content: service.Content{Text: "load"}})
				if err != nil && !errors.Is(err, service.ErrChannelNotFound) {
					t.Errorf("PostMessage: %v", err)
					return
				}
			}
		}()
	}
	for i := range 10 {
		if _, err := c.Reset(ctx, control.ResetOpts{}); err != nil {
			t.Errorf("Reset %d: %v", i, err)
		}
	}
	wg.Wait()
	if _, err := c.Reset(ctx, control.ResetOpts{}); err != nil {
		t.Fatalf("final Reset: %v", err)
	}
	if err := eng.Close(); err != nil { // waits for every delivery goroutine to finish
		t.Fatalf("Close: %v", err)
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.violations) != 0 {
		t.Errorf("attempts for events published before a reset: %v", tr.violations)
	}
	if got, _, _ := eng.Deliveries(events.Filter{}); len(got) != 0 {
		t.Errorf("trace after final reset has %d deliveries, want 0", len(got))
	}
}

func firstHumanUser(t *testing.T, app *storetest.App) service.User {
	t.Helper()
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	for _, u := range us {
		if !u.IsBot {
			return u
		}
	}
	t.Fatal("no human user")
	return service.User{}
}

func generalChannel(t *testing.T, app *storetest.App, caller service.User) string {
	t.Helper()
	page, err := app.Service.ListConversations(t.Context(), caller, service.ListOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	for _, c := range page.Items {
		if c.Name == "general" {
			return c.ID
		}
	}
	t.Fatal("no #general")
	return ""
}
