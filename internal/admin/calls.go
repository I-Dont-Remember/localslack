package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
)

// defaultPollTimeout applies when min is set without timeout_ms.
const defaultPollTimeout = 5 * time.Second

func mountCalls(r chi.Router, d Deps) {
	if d.Calls == nil {
		return
	}
	r.Get("/_localslack/calls", d.listCalls)
	r.Delete("/_localslack/calls", d.clearCalls)
}

func (d Deps) listCalls(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, err := parseCallsQuery(r.URL.Query())
	if err != nil {
		writeError(ctx, d.Log, w, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	res := d.Calls.Query(ctx, q)
	items := res.Calls
	if items == nil {
		items = []calllog.CallRecord{}
	}
	writeJSON(ctx, d.Log, w, http.StatusOK, map[string]any{
		"items": items, "next_since": res.NextSince, "generation": res.Generation,
	})
}

func (d Deps) clearCalls(w http.ResponseWriter, _ *http.Request) {
	d.Calls.Clear()
	w.WriteHeader(http.StatusNoContent)
}

func parseCallsQuery(v url.Values) (calllog.Query, error) {
	q := calllog.Query{
		Method: v.Get("method"), Channel: v.Get("channel"), AppID: v.Get("app_id"), Error: v.Get("error"),
	}
	if s := v.Get("ok"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil || (s != "true" && s != "false") {
			return q, fmt.Errorf("ok must be true or false, got %q", s)
		}
		q.OK = &b
	}
	var errs []error
	since, err := intParam(v, "since")
	errs = append(errs, err)
	limit, err := intParam(v, "limit")
	errs = append(errs, err)
	minN, err := intParam(v, "min")
	errs = append(errs, err)
	timeoutMS, err := intParam(v, "timeout_ms")
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return q, err
	}
	q.Since, q.Limit, q.Min = int64(since), limit, minN
	q.Timeout = time.Duration(timeoutMS) * time.Millisecond
	if minN > 0 && v.Get("timeout_ms") == "" {
		q.Timeout = defaultPollTimeout
	}
	return q, nil
}

// intParam parses an optional non-negative integer query parameter.
func intParam(v url.Values, key string) (int, error) {
	s := v.Get(key)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", key, s)
	}
	return n, nil
}
