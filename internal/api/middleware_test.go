package api_test

import (
	"localslack/constants"
	"localslack/internal/api"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticateToken(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		contentType string
		next        http.Handler
		token       string
		useHeader   bool
		shouldErr   bool
	}{
		{
			name:        "Empty token in JSON content type",
			contentType: constants.CONTENT_TYPE_JSON,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "",
			useHeader:   true,
			shouldErr:   true,
		},
		{
			name:        "Token with 2 characters in JSON content type",
			contentType: constants.CONTENT_TYPE_JSON,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "ab",
			useHeader:   true,
			shouldErr:   true,
		},
		{
			name:        "Valid token in JSON content type",
			contentType: constants.CONTENT_TYPE_JSON,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "abc",
			useHeader:   true,
			shouldErr:   false,
		},
		{
			name:        "Empty token in form content type",
			contentType: constants.CONTENT_TYPE_FORM,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "",
			useHeader:   false,
			shouldErr:   true,
		},
		{
			name:        "Token with 2 characters in form content type",
			contentType: constants.CONTENT_TYPE_FORM,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "ab",
			useHeader:   false,
			shouldErr:   true,
		},
		{
			name:        "Valid token in form content type",
			contentType: constants.CONTENT_TYPE_FORM,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "abc",
			useHeader:   false,
			shouldErr:   false,
		},
		{
			name:        "Ensure form works with header as well for valid token",
			contentType: constants.CONTENT_TYPE_FORM,
			next:        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			token:       "abc",
			useHeader:   true,
			shouldErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", nil)
			r.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			if tt.useHeader {
				r.Header.Set("Authorization", "Bearer "+tt.token)
			} else {
				r.URL.RawQuery = url.Values{"token": []string{tt.token}}.Encode()
			}

			got := api.AuthenticateToken(tt.next)
			got.ServeHTTP(w, r)

			// Check if the expected error occurred
			assert.Equal(t, tt.shouldErr, w.Code != http.StatusOK)
		})
	}
}
