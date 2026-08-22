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

	failedMu sync.Mutex
	failedCh chan struct{}
}

// Go runs fn on its own goroutine, tracked by Wait. It is safe to call Go
// again before a prior call's Wait returns.
func (g *Group) Go(fn func() error) {
	g.wg.Go(func() {
		if err := fn(); err != nil {
			g.once.Do(func() {
				g.err = err
				close(g.failedChannel())
			})
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

// Failed returns a channel that is closed the instant any Go call's fn
// returns a non-nil error -- before Wait itself would return, and even
// while other Go calls tracked by the same group are still running. A
// caller with its own concurrent work can select on this channel alongside
// that work to react to the first failure immediately, instead of only
// discovering it once every tracked goroutine has finished; Err() read
// after a receive from this channel observes that exact first error. It is
// safe to call Failed repeatedly, from multiple goroutines, and before any
// Go call has ever been made.
func (g *Group) Failed() <-chan struct{} {
	return g.failedChannel()
}

// Err returns the first non-nil error recorded so far, or nil if none has
// been recorded yet. It is race-free to call once Failed()'s channel has
// been received from, or after Wait() has returned.
func (g *Group) Err() error {
	return g.err
}

func (g *Group) failedChannel() chan struct{} {
	g.failedMu.Lock()
	defer g.failedMu.Unlock()
	if g.failedCh == nil {
		g.failedCh = make(chan struct{})
	}
	return g.failedCh
}

// Done returns a channel that is closed once every Go call already made on g
// has returned, successfully or not. Unlike Wait, it never blocks the
// calling goroutine: a caller can select on it alongside another signal --
// for example a different Group's Failed() -- to react to whichever
// happens first, then call the now-unblocked Wait() to retrieve the
// aggregated error.
func (g *Group) Done() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	return done
}
