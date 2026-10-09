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

// encodeCursor makes an opaque cursor from a keyset value.
func encodeCursor(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + key))
}

// decodeCursor returns the keyset value; "" decodes to "".
func decodeCursor(c string) (string, error) {
	if c == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	key, ok := strings.CutPrefix(string(b), cursorPrefix)
	if err != nil || !ok || key == "" {
		return "", ErrInvalidCursor
	}
	return key, nil
}

// paginate trims a limit+1 fetch to limit and derives the next cursor from the last kept item.
func paginate[T any](items []T, limit int, key func(T) string) Page[T] {
	if len(items) <= limit {
		return Page[T]{Items: items}
	}
	items = items[:limit]
	return Page[T]{Items: items, NextCursor: encodeCursor(key(items[limit-1]))}
}
