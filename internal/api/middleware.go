package api

import (
	"context"
	"localslack/constants"
	"net/http"
	"strings"
	"unicode/utf8"
)

func AuthenticateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		authHeader := r.Header.Get("Authorization")
		contentType := r.Header.Get("Content-Type")
		if authHeader == "" {
			// legacy option is token is allowed to be a param for form-urlencoded
			if strings.HasPrefix(contentType, constants.CONTENT_TYPE_FORM) {
				tokenParam := r.FormValue("token")
				if tokenParam == "" {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				token = tokenParam
			} else {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// at this point, we got a token from somewhere, and now have to validate it
		// TODO: dummy validation since this isn't that important for now
		if utf8.RuneCountInString(token) <= 2 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "token", token)
		// Call the next handler with the updated context
		// TODO: could consider looking up team & user information based on token here later
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
