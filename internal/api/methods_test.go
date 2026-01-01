package api

import (
	"bytes"
	"encoding/json"
	"io"
	"localslack/internal/api/handlers"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSlackServer_wrap(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		method string
		h      slackHandler
	}{
		{
			name:   "test happy path for JSON handling",
			method: "users.list",
			h:      handlers.HandleUsersList,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer()
			wrappedFunc := s.wrap(tt.method, tt.h)
			r := &http.Request{
				Method: http.MethodPost,
				URL:    &url.URL{Path: "/api/users.list"},
				Header: http.Header{},
			}
			requestBody := bytes.Buffer{}
			r.Body = io.NopCloser(&requestBody)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer YOUR_FAKE_TOKEN")
			rr := httptest.NewRecorder()

			wrappedFunc(rr, r)

			// check we are sending valid JSON back out
			var out map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &out)
			if err != nil {
				t.Errorf("Failed to unmarshal JSON: %v", err)
			}
		})
	}
}
