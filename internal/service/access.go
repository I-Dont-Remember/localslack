// Access rules (membership, private channels, scope gating) land here as one
// canRead/canPost rule set; every write path calls checkAccess.
package service

import (
	"context"

	"localslack/internal/store"
)

// checkAccess is the single hook for per-actor access rules; allows everything for now.
func (s *Service) checkAccess(context.Context, User, store.Conversation) error { return nil }
