package petri

import (
	"fmt"
	"sync"
)

// WorkIDGenerator produces monotonically increasing, human-readable work IDs
// in the format work-{workTypeID}-{N}. It is safe for concurrent use.
type WorkIDGenerator struct {
	mu      sync.Mutex
	counter int
}

// NewWorkIDGenerator creates a WorkIDGenerator starting from zero.
func NewWorkIDGenerator() *WorkIDGenerator {
	return &WorkIDGenerator{}
}

// Next returns the next work ID for the given work type.
// IDs are formatted as work-{workTypeID}-{N} where N is a monotonically
// increasing counter shared across all work types.
func (g *WorkIDGenerator) Next(workTypeID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter++
	return fmt.Sprintf("work-%s-%d", workTypeID, g.counter)
}
