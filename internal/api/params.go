package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"localslack/internal/constants"
)

// parseParams merges query, form body and JSON body into one value set. For
// JSON bodies it also returns the decoded object with types intact.
func parseParams(r *http.Request) (url.Values, map[string]any, error) {
	vals := url.Values{}
	for k, v := range r.URL.Query() {
		vals[k] = v
	}
	var raw map[string]any
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, constants.ContentTypeJSON):
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			return nil, nil, fmt.Errorf("decode json body: %w: %w", errInvalidArguments, err)
		}
		for k, v := range raw {
			vals.Set(k, stringify(v))
		}
	case strings.HasPrefix(ct, constants.ContentTypeForm):
		if err := r.ParseForm(); err != nil {
			return nil, nil, fmt.Errorf("parse form: %w: %w", errInvalidArguments, err)
		}
		for k, v := range r.PostForm {
			vals[k] = v
		}
	}
	return vals, raw, nil
}

// stringify renders a JSON value the way form params would carry it (arrays comma-joined).
func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, stringify(e))
		}
		return strings.Join(parts, ",")
	case nil:
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func csv(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func intParam(v url.Values, key string) int {
	n, err := strconv.Atoi(v.Get(key))
	if err != nil {
		return 0
	}
	return n
}

func boolParam(v url.Values, key string) bool {
	b, _ := strconv.ParseBool(v.Get(key))
	return b
}
