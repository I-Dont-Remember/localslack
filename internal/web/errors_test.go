package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"localslack/internal/service"
)

func TestFailStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want int
	}{
		{service.ErrChannelNotFound, http.StatusNotFound},
		{service.ErrMessageNotFound, http.StatusNotFound},
		{service.ErrIsArchived, http.StatusConflict},
		{service.ErrAlreadyReacted, http.StatusConflict},
		{service.ErrNotInChannel, http.StatusForbidden},
		{service.ErrUserNotInChannel, http.StatusForbidden},
		{service.ErrCantUpdateMessage, http.StatusForbidden},
		{service.ErrCantDeleteMessage, http.StatusForbidden},
		{service.ErrNotAllowedTokenType, http.StatusForbidden},
		{service.ErrMsgTooLong, http.StatusBadRequest},
		{service.ErrInvalidBlocks, http.StatusBadRequest},
		{service.ErrInvalidNamePunctuation, http.StatusBadRequest},
		{service.ErrInvalidReactionName, http.StatusBadRequest},
		{fmt.Errorf("wrapped: %w", service.ErrMsgTooLong), http.StatusBadRequest},
		{fmt.Errorf("boom"), http.StatusInternalServerError},
	}
	w := &Web{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			w.fail(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), tt.err)
			if rec.Code != tt.want {
				t.Errorf("fail(%v) status = %d, want %d", tt.err, rec.Code, tt.want)
			}
		})
	}
}
