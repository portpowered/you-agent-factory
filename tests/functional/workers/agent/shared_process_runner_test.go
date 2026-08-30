package agent_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type agentSharedCommandRouter struct {
	routes map[string]platformprocess.CommandRunner

	mu    sync.Mutex
	calls []agentSharedRoutedCall
}

type agentSharedRoutedCall struct {
	selector string
	request  platformprocess.CommandRequest
}

func newAgentSharedCommandRouter(
	t *testing.T,
	scenarios []agentSharedScenario,
) *agentSharedCommandRouter {
	t.Helper()
	routes := make(map[string]platformprocess.CommandRunner, len(scenarios))
	for _, scenario := range scenarios {
		selector := filepath.Clean(strings.TrimSpace(scenario.factoryDir))
		if selector == "." || selector == "" {
			t.Fatalf("agent scenario %q has empty Factory selector", scenario.name)
		}
		if _, exists := routes[selector]; exists {
			t.Fatalf("duplicate agent Factory selector %q", selector)
		}
		if scenario.runner == nil {
			t.Fatalf("agent scenario %q has no command runner", scenario.name)
		}
		routes[selector] = scenario.runner
	}
	return &agentSharedCommandRouter{routes: routes}
}

func (router *agentSharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	router.mu.Lock()
	runner, ok := router.routes[selector]
	router.mu.Unlock()
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"unknown agent scenario selector %q; refusing to consume another route",
			request.WorkDir,
		)
	}
	router.mu.Lock()
	router.calls = append(router.calls, agentSharedRoutedCall{
		selector: selector,
		request:  cloneAgentCommandRequest(request),
	})
	router.mu.Unlock()
	return runner.Run(ctx, request)
}

func (router *agentSharedCommandRouter) callsFor(selector string) []agentSharedRoutedCall {
	router.mu.Lock()
	defer router.mu.Unlock()
	selector = filepath.Clean(strings.TrimSpace(selector))
	calls := make([]agentSharedRoutedCall, 0)
	for _, call := range router.calls {
		if call.selector != selector {
			continue
		}
		call.request = cloneAgentCommandRequest(call.request)
		calls = append(calls, call)
	}
	return calls
}

func (router *agentSharedCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

func (router *agentSharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *agentSharedCommandRouter) clearRoutes() {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes = nil
}

// agentSharedScenarioRunner gives each immutable Factory-directory route its
// own deterministic command behavior. The cancellation route deliberately
// waits on the invocation context so the public session cancel operation, not
// a timing guess, drives the terminal transition.
type agentSharedScenarioRunner struct {
	behavior agentSharedScenarioBehavior
	result   platformprocess.CommandResult

	started     chan struct{}
	finished    chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	finishOnce  sync.Once
	releaseOnce sync.Once

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	active   atomic.Int32
	canceled atomic.Int32
}

func newAgentSharedScenarioRunner(
	behavior agentSharedScenarioBehavior,
	output string,
	message string,
) *agentSharedScenarioRunner {
	result := platformprocess.CommandResult{Stdout: []byte(output)}
	if behavior == agentSharedFailure {
		result.ExitCode = 1
		result.Stderr = []byte("ERROR: unexpected status 401 Unauthorized {\"type\":\"authentication_error\",\"message\":\"" + message + "\"}")
	}
	if behavior == agentSharedTimeout {
		result.Stderr = []byte(message)
	}
	return &agentSharedScenarioRunner{
		behavior: behavior,
		result:   result,
		started:  make(chan struct{}),
		finished: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (runner *agentSharedScenarioRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer func() {
		runner.active.Add(-1)
		runner.finishOnce.Do(func() { close(runner.finished) })
	}()
	runner.startOnce.Do(func() { close(runner.started) })

	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneAgentCommandRequest(request))
	runner.mu.Unlock()

	switch runner.behavior {
	case agentSharedCancel:
		select {
		case <-ctx.Done():
			runner.canceled.Add(1)
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case agentSharedTimeout:
		return cloneAgentCommandResult(runner.result), context.DeadlineExceeded
	case agentSharedFailure:
		return cloneAgentCommandResult(runner.result), nil
	case agentSharedSuccess:
		return support.NewShapedProviderCommandRunner(runner.result).Run(ctx, request)
	case agentSharedHeldSuccess:
		select {
		case <-runner.release:
			return support.NewShapedProviderCommandRunner(runner.result).Run(ctx, request)
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unknown agent scenario behavior %q", runner.behavior)
	}
}

func (runner *agentSharedScenarioRunner) releaseCall() {
	runner.releaseOnce.Do(func() { close(runner.release) })
}

func (runner *agentSharedScenarioRunner) waitStarted(t testing.TB, timeout time.Duration) {
	t.Helper()
	select {
	case <-runner.started:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for agent command start within %s", timeout)
	}
}

func (runner *agentSharedScenarioRunner) waitFinished(t testing.TB, timeout time.Duration) {
	t.Helper()
	select {
	case <-runner.finished:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for agent command finish within %s", timeout)
	}
}

func (runner *agentSharedScenarioRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *agentSharedScenarioRunner) canceledCount() int {
	return int(runner.canceled.Load())
}

func (runner *agentSharedScenarioRunner) activeCallCount() int {
	return int(runner.active.Load())
}

func cloneAgentCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type agentSharedIdentityGenerator struct {
	sessions      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *agentSharedIdentityGenerator) nextSessionID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *agentSharedIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("agent-shared-response-event-%d", generator.responseEvent.Add(1))
}
