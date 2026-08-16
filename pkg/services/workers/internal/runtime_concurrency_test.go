package internal

import (
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// concurrencyFanout is how many goroutines dispatch concurrently per round in
// TestConcurrentRepeatedExecutionsRetainConstructionInjectedDependencies.
const concurrencyFanout = 20

// concurrencyRepeatRounds is the documented repeat count: the whole
// concurrent fan-out is repeated this many times against the same
// constructed runtime to prove determinism holds under reuse, not just once.
const concurrencyRepeatRounds = 5

// recordingDispatchPublisher is a mutex-guarded ProgressPublisher that groups
// delivered fragments by DispatchID, so a concurrency test can prove no
// fragment crosses from one request's dispatch into another's and none is
// delivered more than the expected number of times.
type recordingDispatchPublisher struct {
	mu        sync.Mutex
	fragments map[string][]workers.ProgressFragment
}

func newRecordingDispatchPublisher() *recordingDispatchPublisher {
	return &recordingDispatchPublisher{fragments: make(map[string][]workers.ProgressFragment)}
}

func (r *recordingDispatchPublisher) publish(fragment workers.ProgressFragment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fragments[fragment.DispatchID] = append(r.fragments[fragment.DispatchID], cloneServiceProgressFragment(fragment))
}

func (r *recordingDispatchPublisher) snapshot() map[string][]workers.ProgressFragment {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][]workers.ProgressFragment, len(r.fragments))
	for id, fragments := range r.fragments {
		out[id] = append([]workers.ProgressFragment(nil), fragments...)
	}
	return out
}
