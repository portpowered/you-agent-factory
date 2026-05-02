package testutil

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// ProviderCommandRunner is a test double for ScriptWrapProvider's shared command seam.
// It records every request and returns queued results in order.
type ProviderCommandRunner struct {
	mu       sync.Mutex
	requests []workers.CommandRequest
	results  []workers.CommandResult
	errors   []error
	index    int
	defaultR workers.CommandResult
}

// NewProviderCommandRunner creates a runner that returns the supplied results in order.
func NewProviderCommandRunner(results ...workers.CommandResult) *ProviderCommandRunner {
	return &ProviderCommandRunner{
		results:  append([]workers.CommandResult(nil), results...),
		defaultR: workers.CommandResult{Stdout: []byte("default mock response")},
	}
}

// Queue appends ordered subprocess results for subsequent Run calls.
func (r *ProviderCommandRunner) Queue(results ...workers.CommandResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.results = append(r.results, results...)
}

// Run records the request and returns the next queued result.
func (r *ProviderCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))

	if r.index < len(r.results) {
		result := r.results[r.index]
		var err error
		if r.index < len(r.errors) {
			err = r.errors[r.index]
		}
		r.index++
		return result, err
	}

	return r.defaultR, nil
}

// Requests returns the recorded command requests in order.
func (r *ProviderCommandRunner) Requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]workers.CommandRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

// CallCount returns how many commands were executed.
func (r *ProviderCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// LastRequest returns the latest recorded command request.
func (r *ProviderCommandRunner) LastRequest() workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.requests) == 0 {
		panic("ProviderCommandRunner: LastRequest() called with no requests")
	}
	return r.requests[len(r.requests)-1]
}

var _ workers.CommandRunner = (*ProviderCommandRunner)(nil)
