package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"localslack/internal/respond"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
)

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
	respond.JSON(ctx, d.Log, w, http.StatusOK, map[string]any{
		"items": items, "next_since": res.NextSince, "generation": res.Generation,
	})
}

func (d Deps) clearCalls(w http.ResponseWriter, _ *http.Request) {
	d.Calls.Clear()
	writeNoContent(w)
}

func parseCallsQuery(v url.Values) (calllog.Query, error) {
	q := calllog.Query{
		Method: v.Get("method"), Channel: v.Get("channel"), AppID: v.Get("app_id"), Error: v.Get("error"),
	}
	if s := v.Get(queryOK); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil || (s != "true" && s != "false") {
			return q, fmt.Errorf("ok must be true or false, got %q", s)
		}
		q.OK = &b
	}
	p, err := parsePoll(v)
	if err != nil {
		return q, err
	}
	q.Since, q.Limit, q.Min, q.Timeout = int64(p.Since), p.Limit, p.Min, p.Timeout
	return q, nil
}
