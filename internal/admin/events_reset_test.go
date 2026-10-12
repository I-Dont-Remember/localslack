package admin_test

import (
	"net/http"
	"testing"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

func TestResetClearsDeliveriesAndKeepsAppsUnlessAll(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	fake := newRecApp(t)
	id := register(t, app.URL, "Resettable", fake.URL, "reaction_added")
	doReq(t, http.MethodPut, app.URL+"/_localslack/delivery-config", constants.ContentTypeJSON, map[string]any{"signature": "stale"})
	doReq(t, http.MethodPost, app.URL+"/_localslack/events", constants.ContentTypeJSON,
		map[string]any{"event": map[string]any{"type": "reaction_added"}})
	_, _, before := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?min=1&timeout_ms=5000", "", nil)
	lastSeq := before["next_since"].(float64)

	status, _, res := doReq(t, http.MethodPost, app.URL+"/_localslack/reset", constants.ContentTypeJSON, map[string]any{})
	if status != http.StatusOK || res["mode"] != "partial" {
		t.Fatalf("reset: %d %v", status, res)
	}
	_, _, after := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries", "", nil)
	if n := len(after["items"].([]any)); n != 0 {
		t.Errorf("deliveries after reset = %d, want 0", n)
	}
	if _, _, cfg := doReq(t, http.MethodGet, app.URL+"/_localslack/delivery-config", "", nil); cfg["signature"] != "valid" {
		t.Errorf("delivery-config after reset = %v, want defaults", cfg)
	}
	if status, _, _ := doReq(t, http.MethodGet, app.URL+"/_localslack/apps/"+id, "", nil); status != http.StatusOK {
		t.Errorf("partial reset removed the app: status %d", status)
	}

	// The dispatcher works again after Resume, and seq did not restart.
	doReq(t, http.MethodPost, app.URL+"/_localslack/events", constants.ContentTypeJSON,
		map[string]any{"event": map[string]any{"type": "reaction_added"}})
	_, _, again := doReq(t, http.MethodGet, app.URL+"/_localslack/deliveries?state=delivered&min=1&timeout_ms=5000", "", nil)
	items := again["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["seq"].(float64) <= lastSeq {
		t.Errorf("deliveries after reset = %v, want one with seq > %v", items, lastSeq)
	}

	if status, _, _ := doReq(t, http.MethodPost, app.URL+"/_localslack/reset?all=true", constants.ContentTypeJSON, map[string]any{}); status != http.StatusOK {
		t.Fatalf("reset all status %d", status)
	}
	if status, _, _ := doReq(t, http.MethodGet, app.URL+"/_localslack/apps/"+id, "", nil); status != http.StatusNotFound {
		t.Errorf("reset?all=true kept the app: status %d", status)
	}
}
