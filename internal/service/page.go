package service

import (
	"encoding/base64"
	"strings"
)

const (
	defaultLimit = 100
	maxLimit     = 1000
	cursorPrefix = "v1:"
)

// Cursor kinds bind a cursor to the endpoint that issued it.
const (
	cursorConversations = "c"
	cursorHistory       = "h"
	cursorReplies       = "r"
)

// Page is one page of results; NextCursor is empty on the last page.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

// PageOpts is cursor pagination input. Limit <= 0 means the default.
type PageOpts struct {
	Cursor string
	Limit  int
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	}
	return n
}

// encodeCursor makes an opaque cursor from a keyset value for one endpoint kind.
func encodeCursor(kind, key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + kind + ":" + key))
}

// decodeCursor returns the keyset value; "" decodes to "". A cursor issued
// for a different endpoint kind is invalid.
func decodeCursor(kind, c string) (string, error) {
	if c == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	rest, ok := strings.CutPrefix(string(b), cursorPrefix+kind+":")
	if err != nil || !ok || rest == "" {
		return "", ErrInvalidCursor
	}
	return rest, nil
}

// paginate trims a limit+1 fetch to limit and derives the next cursor from the last kept item.
func paginate[T any](kind string, items []T, limit int, key func(T) string) Page[T] {
	if len(items) <= limit {
		return Page[T]{Items: items}
	}
	items = items[:limit]
	return Page[T]{Items: items, NextCursor: encodeCursor(kind, key(items[limit-1]))}
}
