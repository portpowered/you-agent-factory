// Package taskgroup owns a policy-free primitive for running functions
// concurrently and collecting their outcome, so a caller never needs to
// declare its own goroutine, WaitGroup, or once-only error latch just to run
// a handful of independent calls in parallel and learn whether any of them
// failed.
package taskgroup

import "sync"

// Group runs functions concurrently on their own goroutines and waits for
// all of them, keeping only the first non-nil error any of them returns --
// the same "every call still runs to completion; the first failure wins"
// shape golang.org/x/sync/errgroup.Group offers, without that package's
// context-cancellation behavior, which a caller that only needs to track
// completion and a first error does not need.
type Group struct {
	wg   sync.WaitGroup
	once sync.Once
	err  error
}

// Go runs fn on its own goroutine, tracked by Wait. It is safe to call Go
// again before a prior call's Wait returns.
func (g *Group) Go(fn func() error) {
	g.wg.Go(func() {
		if err := fn(); err != nil {
			g.once.Do(func() { g.err = err })
		}
	})
}

// Wait blocks until every Go call that happened before it has returned, then
// reports the first non-nil error any of them returned, or nil. It is safe
// to call more than once, including concurrently with a later Go call: a
// later Go call's own outcome is only guaranteed to be reflected by a Wait
// call that starts after it.
func (g *Group) Wait() error {
	g.wg.Wait()
	return g.err
}
