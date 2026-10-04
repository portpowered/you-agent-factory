package acceptance

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type invokeContinueStaticCommandRouteEntry struct {
	workingDirectory string
	runner           platformprocess.CommandRunner
}

type invokeContinueStaticCommandRoute struct {
	mu          sync.RWMutex
	routes      []invokeContinueStaticCommandRouteEntry
	requestLog  []platformprocess.CommandRequest
	activeCalls atomic.Int32
}

func (route *invokeContinueStaticCommandRoute) Close() {
	if route == nil {
		return
	}
	route.mu.Lock()
	route.routes = nil
	route.mu.Unlock()
}

func (route *invokeContinueStaticCommandRoute) routeCount() int {
	if route == nil {
		return 0
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	return len(route.routes)
}

func (route *invokeContinueStaticCommandRoute) activeCallCount() int {
	if route == nil {
		return 0
	}
	return int(route.activeCalls.Load())
}

func (route *invokeContinueStaticCommandRoute) requests() []platformprocess.CommandRequest {
	if route == nil {
		return nil
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	requests := make([]platformprocess.CommandRequest, len(route.requestLog))
	for index, request := range route.requestLog {
		requests[index] = cloneS8CommandRequest(request)
	}
	return requests
}

func (route *invokeContinueStaticCommandRoute) recordRequest(request platformprocess.CommandRequest) {
	route.mu.Lock()
	route.requestLog = append(route.requestLog, cloneS8CommandRequest(request))
	route.mu.Unlock()
}

// invokeContinueResettableProviderCommandRunner keeps the immutable route
// stable while allowing -count repetitions to receive a fresh ordered ledger.
// The package fixture still chooses this runner only from the pre-registered
// WorkDir route; Reset is test-process reuse, not runtime route mutation.
type invokeContinueResettableProviderCommandRunner struct {
	mu      sync.RWMutex
	results []platformprocess.CommandResult
	runner  *testutil.ProviderCommandRunner
}

func newInvokeContinueResettableProviderCommandRunner(
	results ...platformprocess.CommandResult,
) *invokeContinueResettableProviderCommandRunner {
	runner := &invokeContinueResettableProviderCommandRunner{
		results: append([]platformprocess.CommandResult(nil), results...),
	}
	runner.Reset()
	return runner
}

func (runner *invokeContinueResettableProviderCommandRunner) Reset() {
	runner.mu.Lock()
	runner.runner = testutil.NewProviderCommandRunner(runner.results...)
	runner.mu.Unlock()
}

func (runner *invokeContinueResettableProviderCommandRunner) current() *testutil.ProviderCommandRunner {
	runner.mu.RLock()
	defer runner.mu.RUnlock()
	return runner.runner
}

func (runner *invokeContinueResettableProviderCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return runner.current().Run(ctx, request)
}

func (runner *invokeContinueResettableProviderCommandRunner) CallCount() int {
	return runner.current().CallCount()
}

func (runner *invokeContinueResettableProviderCommandRunner) Requests() []platformprocess.CommandRequest {
	return runner.current().Requests()
}

var _ invokeContinueProviderCommandRunner = (*invokeContinueResettableProviderCommandRunner)(nil)

var _ platformprocess.CommandRunner = (*invokeContinueResettableProviderCommandRunner)(nil)

// Run selects only a route fixed before process construction. It deliberately
// has no mutable map, request-order fallback, or Factory Session lookup.
func (route *invokeContinueStaticCommandRoute) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.activeCalls.Add(1)
	defer route.activeCalls.Add(-1)
	route.recordRequest(request)
	entry, err := route.entry(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	return entry.runner.Run(ctx, request)
}

// RunStreaming preserves the optional streaming capability of a scenario
// runner while retaining the same immutable WorkDir-only route selection.
// Provider adapters use this extension for live Worker Session observations;
// a non-streaming scenario falls back to one completed chunk per stream.
func (route *invokeContinueStaticCommandRoute) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.activeCalls.Add(1)
	defer route.activeCalls.Add(-1)
	route.recordRequest(request)
	entry, err := route.entry(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	if streaming, ok := entry.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, runErr := entry.runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, runErr
}

func (route *invokeContinueStaticCommandRoute) entry(
	request platformprocess.CommandRequest,
) (invokeContinueStaticCommandRouteEntry, error) {
	if route == nil {
		return invokeContinueStaticCommandRouteEntry{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	for _, entry := range route.routes {
		if filepath.Clean(request.WorkDir) != filepath.Clean(entry.workingDirectory) {
			continue
		}
		if entry.runner == nil {
			return invokeContinueStaticCommandRouteEntry{}, fmt.Errorf("invoke/continue provider route for WorkDir %q is unavailable", request.WorkDir)
		}
		return entry, nil
	}
	return invokeContinueStaticCommandRouteEntry{}, fmt.Errorf("no invoke/continue provider route matched WorkDir %q", request.WorkDir)
}

var _ platformprocess.CommandRunner = (*invokeContinueStaticCommandRoute)(nil)
