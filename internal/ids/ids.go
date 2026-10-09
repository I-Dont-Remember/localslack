// Package ids generates Slack-style identifiers (T..., U..., C..., D...).
package ids

import (
	"hash/fnv"
	"math/rand/v2"
	"sync"
)

const (
	alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bodyLen  = 10
)

// Generator produces ids from a seeded RNG, so the same seed yields the same ids.
type Generator struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewGenerator returns a Generator seeded from an arbitrary string.
func NewGenerator(seed string) *Generator {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	return &Generator{rng: rand.New(rand.NewPCG(h.Sum64(), 0))}
}

// Team returns a team id (T...).
func (g *Generator) Team() string { return g.next('T') }

// User returns a user id (U...).
func (g *Generator) User() string { return g.next('U') }

// Channel returns a channel id (C...).
func (g *Generator) Channel() string { return g.next('C') }

// DM returns a direct-message conversation id (D...).
func (g *Generator) DM() string { return g.next('D') }

// Group returns a group-DM (mpim) conversation id (G...).
func (g *Generator) Group() string { return g.next('G') }

func (g *Generator) next(prefix byte) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	b := make([]byte, 1+bodyLen)
	b[0] = prefix
	for i := 1; i < len(b); i++ {
		b[i] = alphabet[g.rng.IntN(len(alphabet))]
	}
	return string(b)
}
