package faults_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/faults"
)

func errAction() faults.Action { return faults.Action{Status: 500} }

func fire(t *testing.T, s *faults.Set, n int, req faults.Request) string {
	t.Helper()
	var b strings.Builder
	for range n {
		if _, ok := s.Intercept(t.Context(), req); ok {
			b.WriteByte('X')
		} else {
			b.WriteByte('.')
		}
	}
	return b.String()
}

func TestCounting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec faults.Spec
		want string
	}{
		{"always", faults.Spec{Method: "m", Action: errAction()}, "XXXXXX"},
		{"nth=3 only third", faults.Spec{Method: "m", Nth: 3, Action: errAction()}, "..X..."},
		{"nth=3 times=2", faults.Spec{Method: "m", Nth: 3, Times: 2, Action: errAction()}, "..XX.."},
		{"every=2", faults.Spec{Method: "m", Every: 2, Action: errAction()}, ".X.X.X"},
		{"every=2 times=2", faults.Spec{Method: "m", Every: 2, Times: 2, Action: errAction()}, ".X.X.."},
		{"nth=2 every=3", faults.Spec{Method: "m", Nth: 2, Every: 3, Action: errAction()}, ".X..X."},
		{"times=2", faults.Spec{Method: "m", Times: 2, Action: errAction()}, "XX...."},
		{"once", faults.Spec{Method: "m", Once: true, Action: errAction()}, "X....."},
		{"wildcard", faults.Spec{Method: "*", Nth: 2, Action: errAction()}, ".X...."},
		{"other method never", faults.Spec{Method: "other", Action: errAction()}, "......"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := faults.NewSet()
			if _, err := s.Add(tt.spec); err != nil {
				t.Fatalf("Add: %v", err)
			}

			got := fire(t, s, 6, faults.Request{Method: "m"})

			if got != tt.want {
				t.Errorf("pattern = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	ch := func(id string) url.Values { return url.Values{"channel": {id}} }
	tests := []struct {
		name  string
		match faults.Match
		req   faults.Request
		want  string
	}{
		{"channel hit", faults.Match{Channel: "C1"}, faults.Request{Method: "m", Params: ch("C1")}, "X"},
		{"channel miss", faults.Match{Channel: "C1"}, faults.Request{Method: "m", Params: ch("C2")}, "."},
		{"channel absent", faults.Match{Channel: "C1"}, faults.Request{Method: "m"}, "."},
		{"app hit", faults.Match{AppID: "A1"}, faults.Request{Method: "m", AppID: "A1"}, "X"},
		{"app miss", faults.Match{AppID: "A1"}, faults.Request{Method: "m", AppID: "A2"}, "."},
		{"user hit", faults.Match{UserID: "U1"}, faults.Request{Method: "m", UserID: "U1"}, "X"},
		{"token kind miss", faults.Match{TokenKind: "bot"}, faults.Request{Method: "m", TokenKind: "user"}, "."},
		{"all fields", faults.Match{Channel: "C1", AppID: "A1"}, faults.Request{Method: "m", AppID: "A1", Params: ch("C1")}, "X"},
		{"one of two misses", faults.Match{Channel: "C1", AppID: "A1"}, faults.Request{Method: "m", AppID: "A2", Params: ch("C1")}, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := faults.NewSet()
			if _, err := s.Add(faults.Spec{Method: "m", Match: tt.match, Action: errAction()}); err != nil {
				t.Fatalf("Add: %v", err)
			}

			got := fire(t, s, 1, tt.req)

			if got != tt.want {
				t.Errorf("pattern = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNonMatchingCallsDoNotCount(t *testing.T) {
	t.Parallel()
	s := faults.NewSet()
	if _, err := s.Add(faults.Spec{Method: "m", Match: faults.Match{Channel: "C1"}, Nth: 2, Action: errAction()}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c1 := faults.Request{Method: "m", Params: url.Values{"channel": {"C1"}}}
	c2 := faults.Request{Method: "m", Params: url.Values{"channel": {"C2"}}}

	got := fire(t, s, 3, c2) + fire(t, s, 1, c1) + fire(t, s, 1, c2) + fire(t, s, 1, c1)

	if want := ".....X"; got != want {
		t.Errorf("pattern = %s, want %s", got, want)
	}
}

func TestOrderingFirstFiringWins(t *testing.T) {
	t.Parallel()
	s := faults.NewSet()
	first, _ := s.Add(faults.Spec{Method: "m", Nth: 2, Action: faults.Action{Status: 500}})
	second, _ := s.Add(faults.Spec{Method: "*", Action: faults.Action{Status: 503}})
	var got []string
	for range 3 {
		ov, ok := s.Intercept(t.Context(), faults.Request{Method: "m"})
		if !ok {
			t.Fatal("expected a fault on every call")
		}
		got = append(got, ov.FaultID)
	}

	want := []string{second.ID, first.ID, second.ID}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fault ids (-want +got):\n%s", diff)
	}
	list := s.List()
	if list[0].Seen != 2 || list[0].Fired != 1 || list[1].Seen != 3 || list[1].Fired != 2 {
		t.Errorf("counters = %+v, want rule1 seen2/fired1 (spent after nth), rule2 seen3/fired2", list)
	}
}

func TestSpentRuleStopsMatchingAndFallsThrough(t *testing.T) {
	t.Parallel()
	s := faults.NewSet()
	a, _ := s.Add(faults.Spec{Method: "m", Once: true, Action: faults.Action{Status: 500}})
	b, _ := s.Add(faults.Spec{Method: "m", Action: faults.Action{Status: 503}})

	var got []string
	for range 3 {
		ov, _ := s.Intercept(t.Context(), faults.Request{Method: "m"})
		got = append(got, ov.FaultID)
	}

	want := []string{a.ID, b.ID, b.ID}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fault ids (-want +got):\n%s", diff)
	}
	if l := s.List(); !l[0].Spent || l[0].Seen != 1 || l[1].Spent {
		t.Errorf("list = %+v, want rule1 spent with seen=1", l)
	}
}

func TestIDsListDeleteClear(t *testing.T) {
	t.Parallel()
	s := faults.NewSet()
	a, _ := s.Add(faults.Spec{Method: "m", Action: errAction()})
	b, _ := s.Add(faults.Spec{Method: "m", Action: errAction()})
	if a.ID != "f_1" || b.ID != "f_2" {
		t.Errorf("ids = %s, %s; want f_1, f_2", a.ID, b.ID)
	}

	if !s.Delete(a.ID) || s.Delete(a.ID) {
		t.Error("Delete: want true then false")
	}
	if got := len(s.List()); got != 1 {
		t.Errorf("len(List) = %d, want 1", got)
	}
	s.Clear()
	if got := len(s.List()); got != 0 {
		t.Errorf("len(List) after Clear = %d, want 0", got)
	}
	c, _ := s.Add(faults.Spec{Method: "m", Action: errAction()})
	if c.ID != "f_3" {
		t.Errorf("id after clear = %s, want f_3 (ids never reused)", c.ID)
	}
}

func TestOverride(t *testing.T) {
	t.Parallel()

	rl := func(secs string) http.Header { return http.Header{"Retry-After": {secs}} }
	tests := []struct {
		name   string
		action faults.Action
		want   faults.Override
	}{
		{
			"429 default",
			faults.Action{Status: 429},
			faults.Override{Respond: true, Status: 429, Headers: rl("1"), Body: map[string]any{"ok": false, "error": "ratelimited"}},
		},
		{
			"429 retry_after",
			faults.Action{Status: 429, RetryAfter: 7},
			faults.Override{Respond: true, Status: 429, Headers: rl("7"), Body: map[string]any{"ok": false, "error": "ratelimited"}},
		},
		{
			"500 default",
			faults.Action{Status: 500},
			faults.Override{Respond: true, Status: 500, Body: map[string]any{"ok": false, "error": "internal_error"}},
		},
		{
			"503 custom body",
			faults.Action{Status: 503, Body: map[string]any{"x": 1}},
			faults.Override{Respond: true, Status: 503, Body: map[string]any{"x": 1}},
		},
		{
			"200 slack error",
			faults.Action{Error: "channel_not_found"},
			faults.Override{Respond: true, Status: 200, Body: map[string]any{"ok": false, "error": "channel_not_found"}},
		},
		{
			"delay only",
			faults.Action{DelayMS: 250},
			faults.Override{Delay: 250 * time.Millisecond},
		},
		{
			"delay and error",
			faults.Action{DelayMS: 5, Error: "fatal_error"},
			faults.Override{Delay: 5 * time.Millisecond, Respond: true, Status: 200, Body: map[string]any{"ok": false, "error": "fatal_error"}},
		},
		{
			"400 custom error",
			faults.Action{Status: 400, Error: "bad"},
			faults.Override{Respond: true, Status: 400, Body: map[string]any{"ok": false, "error": "bad"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := faults.NewSet()
			r, err := s.Add(faults.Spec{Method: "m", Action: tt.action})
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			tt.want.FaultID = r.ID

			got, ok := s.Intercept(t.Context(), faults.Request{Method: "m"})

			if !ok {
				t.Fatal("not fired")
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("override (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		spec        faults.Spec
		wantPointer string
	}{
		{"no method", faults.Spec{Action: errAction()}, "/method"},
		{"empty action", faults.Spec{Method: "m"}, "/action"},
		{"status 200 alone", faults.Spec{Method: "m", Action: faults.Action{Status: 200}}, "/action"},
		{"status 302", faults.Spec{Method: "m", Action: faults.Action{Status: 302}}, "/action/status"},
		{"status 600", faults.Spec{Method: "m", Action: faults.Action{Status: 600}}, "/action/status"},
		{"delay too long", faults.Spec{Method: "m", Action: faults.Action{DelayMS: 60001}}, "/action/delay_ms"},
		{"negative delay", faults.Spec{Method: "m", Action: faults.Action{DelayMS: -1, Status: 500}}, "/action/delay_ms"},
		{"negative retry_after", faults.Spec{Method: "m", Action: faults.Action{Status: 429, RetryAfter: -1}}, "/action/retry_after"},
		{"negative nth", faults.Spec{Method: "m", Nth: -1, Action: errAction()}, "/nth"},
		{"negative every", faults.Spec{Method: "m", Every: -1, Action: errAction()}, "/every"},
		{"negative times", faults.Spec{Method: "m", Times: -1, Action: errAction()}, "/times"},
		{"once with times", faults.Spec{Method: "m", Once: true, Times: 3, Action: errAction()}, "/once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := faults.NewSet()

			_, err := s.Add(tt.spec)

			var ve *faults.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			found := false
			for _, d := range ve.Details {
				found = found || d.Pointer == tt.wantPointer
			}
			if !found {
				t.Errorf("details = %+v, want pointer %s", ve.Details, tt.wantPointer)
			}
			if got := len(s.List()); got != 0 {
				t.Errorf("rules after invalid Add = %d, want 0", got)
			}
		})
	}
}

func TestDelayCapAllowed(t *testing.T) {
	t.Parallel()
	if _, err := faults.NewSet().Add(faults.Spec{Method: "m", Action: faults.Action{DelayMS: 60000}}); err != nil {
		t.Errorf("Add at cap: %v", err)
	}
}

func TestConcurrentIntercept(t *testing.T) {
	t.Parallel()
	s := faults.NewSet()
	if _, err := s.Add(faults.Spec{Method: "m", Times: 10, Action: errAction()}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	fired := 0
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if _, ok := s.Intercept(t.Context(), faults.Request{Method: "m"}); ok {
					mu.Lock()
					fired++
					mu.Unlock()
				}
				_ = s.List()
			}
		}()
	}
	wg.Wait()

	if fired != 10 {
		t.Errorf("fired = %d, want exactly 10", fired)
	}
	if l := s.List()[0]; l.Seen != 10 || l.Fired != 10 {
		t.Errorf("counters = %+v, want seen=fired=10 (spent rules stop counting)", l)
	}
}
