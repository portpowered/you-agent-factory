package petri

import (
	"fmt"
	"math/big"
	"strings"
	"sync"
)

// WorkIDGenerator produces monotonically increasing, human-readable work IDs
// in the format work-{workTypeID}-{N}. It is safe for concurrent use.
type WorkIDGenerator struct {
	mu      sync.Mutex
	counter big.Int
}

// NewWorkIDGenerator creates a WorkIDGenerator after reserving the numeric
// sequence used by previously admitted or emitted Work identities.
func NewWorkIDGenerator(existingWorkIDs ...string) *WorkIDGenerator {
	generator := &WorkIDGenerator{}
	generator.Reserve(existingWorkIDs...)
	return generator
}

// Reserve advances the generator beyond every parseable generated Work ID.
// It is safe to call concurrently with Next, although restore seeds the
// generator before the runtime is exposed to new submissions.
func (g *WorkIDGenerator) Reserve(workIDs ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, workID := range workIDs {
		counter, ok := generatedWorkIDCounter(workID)
		if ok && counter.Cmp(&g.counter) > 0 {
			g.counter.Set(counter)
		}
	}
}

// Next returns the next work ID for the given work type.
// IDs are formatted as work-{workTypeID}-{N} where N is a monotonically
// increasing counter shared across all work types.
func (g *WorkIDGenerator) Next(workTypeID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter.Add(&g.counter, big.NewInt(1))
	return fmt.Sprintf("work-%s-%s", workTypeID, g.counter.String())
}

func generatedWorkIDCounter(workID string) (*big.Int, bool) {
	if !strings.HasPrefix(workID, "work-") {
		return nil, false
	}
	separator := strings.LastIndexByte(workID, '-')
	if separator <= len("work-") || separator == len(workID)-1 {
		return nil, false
	}
	counter, ok := new(big.Int).SetString(workID[separator+1:], 10)
	if !ok || counter.Sign() < 1 {
		return nil, false
	}
	return counter, true
}
