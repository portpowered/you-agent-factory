//go:build factoryartifact

package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type resumedResponseScopeProviderRunner struct {
	gate         chan struct{}
	release      sync.Once
	mu           sync.Mutex
	requests     []platformprocess.CommandRequest
	callSignal   chan struct{}
	returnSignal chan struct{}
	returns      int
}

func newResumedResponseScopeProviderRunner(gate chan struct{}) *resumedResponseScopeProviderRunner {
	return &resumedResponseScopeProviderRunner{
		gate: gate, callSignal: make(chan struct{}, 128), returnSignal: make(chan struct{}, 128),
	}
}

func (runner *resumedResponseScopeProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneResumedCommandRequest(request))
	runner.mu.Unlock()
	select {
	case runner.callSignal <- struct{}{}:
	default:
	}
	if !strings.EqualFold(strings.TrimSpace(request.Command), string(modelprovider.ProviderCodex)) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider command %q", request.Command)
	}
	result := platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(resumedResponseScopeControlledOutput)}
	select {
	case <-runner.gate:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	runner.recordReturn()
	return result, nil
}

func (runner *resumedResponseScopeProviderRunner) WaitForCall(ctx context.Context) error {
	for {
		runner.mu.Lock()
		calls := len(runner.requests)
		runner.mu.Unlock()
		if calls > 0 {
			return nil
		}
		select {
		case <-runner.callSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *resumedResponseScopeProviderRunner) WaitForCallCount(ctx context.Context, want int) error {
	for {
		if runner.CallCount() >= want {
			return nil
		}
		select {
		case <-runner.callSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) ReturnCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.returns
}

func (runner *resumedResponseScopeProviderRunner) WaitForReturnCount(ctx context.Context, want int) error {
	for {
		if runner.ReturnCount() >= want {
			return nil
		}
		select {
		case <-runner.returnSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) recordReturn() {
	runner.mu.Lock()
	runner.returns++
	runner.mu.Unlock()
	select {
	case runner.returnSignal <- struct{}{}:
	default:
	}
}

func (runner *resumedResponseScopeProviderRunner) WorkDirs() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	result := make([]string, 0, len(runner.requests))
	for _, request := range runner.requests {
		result = append(result, request.WorkDir)
	}
	return result
}

func (runner *resumedResponseScopeProviderRunner) Release() {
	runner.release.Do(func() { close(runner.gate) })
}

func (runner *resumedResponseScopeProviderRunner) AssertSafeRequests(t testing.TB) {
	t.Helper()
	runner.mu.Lock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index := range runner.requests {
		requests[index] = cloneResumedCommandRequest(runner.requests[index])
	}
	runner.mu.Unlock()
	if len(requests) == 0 {
		t.Fatal("controlled ProviderCommandRunner observed no requests")
	}
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal controlled provider request: %v", err)
		}
		if strings.Contains(string(encoded), resumedResponseScopeSecretMarker) {
			t.Fatalf("controlled provider request leaked rejected Work payload marker")
		}
	}
}

func cloneResumedCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*resumedResponseScopeProviderRunner)(nil)
