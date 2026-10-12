package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

type stateBody struct {
	OK            bool `json:"ok"`
	Conversations []struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Kind     string   `json:"kind"`
		Members  []string `json:"members"`
		Messages []struct {
			TS   string `json:"ts"`
			Text string `json:"text"`
		} `json:"messages"`
	} `json:"conversations"`
}

func getState(t *testing.T, app *storetest.App) stateBody {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, app.URL+"/_localslack/state/conversations", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out stateBody
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestStateConversations(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	caller := us[0]

	before := getState(t, app)
	var generalID string
	var seeded int
	for _, c := range before.Conversations {
		if c.Name == "general" {
			generalID, seeded = c.ID, len(c.Messages)
			if c.Kind != "channel" || len(c.Members) != len(us) {
				t.Errorf("general = %+v, want channel with %d members", c, len(us))
			}
		}
	}
	if !before.OK || generalID == "" || seeded < 3 {
		t.Fatalf("state = %+v, want ok with seeded #general (>=3 messages)", before)
	}

	if _, err := app.Service.PostMessage(t.Context(), caller, service.PostOpts{Channel: generalID, Content: service.Content{Text: "visible in state"}}); err != nil {
		t.Fatalf("post: %v", err)
	}
	after := getState(t, app)
	for _, c := range after.Conversations {
		if c.ID != generalID {
			continue
		}
		if len(c.Messages) != seeded+1 || c.Messages[len(c.Messages)-1].Text != "visible in state" {
			t.Errorf("general messages after post = %+v, want %d ending in the new one", c.Messages, seeded+1)
		}
	}
}
