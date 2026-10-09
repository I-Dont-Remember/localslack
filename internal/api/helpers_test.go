package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

// call POSTs a form-encoded Slack method with an optional bearer token and decodes the JSON body.
func call(t *testing.T, app *storetest.App, method, token string, params map[string]string) (int, map[string]any) {
	t.Helper()
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	return do(t, http.MethodPost, app.URL+"/api/"+method, token, constants.ContentTypeForm, strings.NewReader(form.Encode()))
}

func do(t *testing.T, verb, target, token, contentType string, body *strings.Reader) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), verb, target, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return resp.StatusCode, out
}
