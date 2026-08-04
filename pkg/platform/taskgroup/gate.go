package taskgroup

import "sync"

// Gate coordinates a producer that keeps issuing new units of concurrent
// work against a consumer that wants to guarantee, deterministically, that
// no new unit is issued once it decides to stop -- without blocking
// indefinitely on a producer that may never issue another unit at all. Its
// zero value is ready to use.
type Gate struct {
	mu     sync.RWMutex
	closed bool
}

// Enter reports whether the caller may proceed with one unit of gated work.
// When it returns true, the caller must call release exactly once, as soon
// as it has finished issuing (not necessarily completing) that unit of
// work -- for example, immediately after handing a task to a Group.Go call,
// not after that task itself finishes. When it returns false, g is already
// closed, release is nil, and the caller must not proceed with the work it
// was about to issue.
func (g *Gate) Enter() (release func(), ok bool) {
	g.mu.RLock()
	if g.closed {
		g.mu.RUnlock()
		return nil, false
	}
	return g.mu.RUnlock, true
}

// Close blocks until every Enter call that already returned true has had
// its release func called, then permanently closes g so every future Enter
// call returns false immediately, without blocking. It is safe to call more
// than once; only the first call has any effect.
func (g *Gate) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}
