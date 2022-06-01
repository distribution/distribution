package cidutil

import (
	"context"

	c "github.com/ipfs/go-cid"
)

type Set = c.Set

func NewSet() *Set { return c.NewSet() }

// StreamingSet is an extension of Set which allows to implement back-pressure
// for the Visit function
type StreamingSet struct {
	Set *Set
	New chan c.Cid
}

// NewStreamingSet initializes and returns new Set.
func NewStreamingSet() *StreamingSet {
	return &StreamingSet{
		Set: c.NewSet(),
		New: make(chan c.Cid),
	}
}

// Visitor creates new visitor which adds a Cids to the set and emits them to
// the set.New channel
func (s *StreamingSet) Visitor(ctx context.Context) func(c c.Cid) bool {
	return func(c c.Cid) bool {
		if s.Set.Visit(c) {
			select {
			case s.New <- c:
			case <-ctx.Done():
			}
			return true
		}

		return false
	}
}
