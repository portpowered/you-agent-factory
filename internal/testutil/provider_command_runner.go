package testutil

import (
	"context"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// ProviderCommandRunner is a test double for ScriptWrapProvider's shared command seam.
// It records every request and returns queued results in order.
type ProviderCommandRunner struct {
	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	results  []platformprocess.CommandResult
	errors   []error
	index    int
	defaultR platformprocess.CommandResult
}

// NewProviderCommandRunner creates a runner that returns the supplied results in order.
func NewProviderCommandRunner(results ...platformprocess.CommandResult) *ProviderCommandRunner {
	return &ProviderCommandRunner{
		results:  append([]platformprocess.CommandResult(nil), results...),
		defaultR: platformprocess.CommandResult{Stdout: []byte("default mock response")},
	}
}

// Queue appends ordered subprocess results for subsequent Run calls.
func (r *ProviderCommandRunner) Queue(results ...platformprocess.CommandResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.results = append(r.results, results...)
}

// Run records the request and returns the next queued result.
func (r *ProviderCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, cloneProcessRequest(req))

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
func (r *ProviderCommandRunner) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]platformprocess.CommandRequest, len(r.requests))
	for index, request := range r.requests {
		out[index] = cloneProcessRequest(request)
	}
	return out
}

// CallCount returns how many commands were executed.
func (r *ProviderCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// LastRequest returns the latest recorded command request.
func (r *ProviderCommandRunner) LastRequest() platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.requests) == 0 {
		panic("ProviderCommandRunner: LastRequest() called with no requests")
	}
	return cloneProcessRequest(r.requests[len(r.requests)-1])
}

func cloneProcessRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*ProviderCommandRunner)(nil)
