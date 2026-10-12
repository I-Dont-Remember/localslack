package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"localslack/internal/api"
	"localslack/internal/calllog"
	"localslack/internal/faults"
)

func TestInterceptorDelayStopsOnCancel(t *testing.T) {
	t.Parallel()

	icpt := &override{ov: faults.Override{FaultID: "F3", Delay: time.Hour}, ok: true, got: make(chan faults.Request, 1)}
	s := &sink{}
	srv := pipeline(t, api.WithInterceptor(icpt), api.WithRecorder(s))
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/auth.test", strings.NewReader(""))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Authorization", "Bearer xoxb-x")

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request succeeded, want client timeout during delay")
	}

	rec := waitOneRecord(t, s)
	if rec.FaultID != "F3" || rec.Response != nil {
		t.Errorf("record = %+v, want fault F3 and no response (method never ran)", rec)
	}
}

// waitOneRecord waits (bounded) for the server to finish the cancelled call.
func waitOneRecord(t *testing.T, s *sink) calllog.CallRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		recs := append([]calllog.CallRecord(nil), s.recs...)
		s.mu.Unlock()
		if len(recs) == 1 {
			return recs[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("handler did not return after client cancel")
	return calllog.CallRecord{}
}
