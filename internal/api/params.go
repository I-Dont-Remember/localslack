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

// parseParams merges query, form body and JSON body into one value set.
func parseParams(r *http.Request) (url.Values, error) {
	vals := url.Values{}
	for k, v := range r.URL.Query() {
		vals[k] = v
	}
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, constants.ContentTypeJSON):
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("decode json body: %w", err)
		}
		for k, v := range m {
			vals.Set(k, stringify(v))
		}
	case strings.HasPrefix(ct, constants.ContentTypeForm):
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("parse form: %w", err)
		}
		for k, v := range r.PostForm {
			vals[k] = v
		}
	}
	return vals, nil
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
