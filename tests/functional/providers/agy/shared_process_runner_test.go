package agy

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type agySharedCommandOutcome struct {
	result  platformprocess.CommandResult
	err     error
	release <-chan struct{}
}

// agySharedCommandRoute is immutable after the package-owned command router
// freezes. Its request ledger is invocation-observable and remains separate
// from the other routes' ledgers.
type agySharedCommandRoute struct {
	selector    string
	workDir     string
	rootDir     string
	homeDir     string
	assetPath   string
	factoryName string
	outcomes    []agySharedCommandOutcome

	mu             sync.Mutex
	requests       []platformprocess.CommandRequest
	recordingPaths []string
}

func (route *agySharedCommandRoute) record(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	route.mu.Lock()
	index := len(route.requests)
	route.requests = append(route.requests, cloneAgyCommandRequest(request))
	outcome := route.outcome(index)
	route.mu.Unlock()

	if outcome.release != nil {
		select {
		case <-outcome.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return cloneAgyCommandResult(outcome.result), outcome.err
}

func (route *agySharedCommandRoute) outcome(index int) agySharedCommandOutcome {
	if len(route.outcomes) == 0 {
		return agySharedCommandOutcome{}
	}
	if index >= len(route.outcomes) {
		index = len(route.outcomes) - 1
	}
	return route.outcomes[index]
}

func (route *agySharedCommandRoute) callCount() int {
	route.mu.Lock()
	defer route.mu.Unlock()
	return len(route.requests)
}

func (route *agySharedCommandRoute) lastRequest() platformprocess.CommandRequest {
	route.mu.Lock()
	defer route.mu.Unlock()
	if len(route.requests) == 0 {
		panic("agySharedCommandRoute: LastRequest called with no requests")
	}
	return cloneAgyCommandRequest(route.requests[len(route.requests)-1])
}

func (route *agySharedCommandRoute) recordRecordingPath(path string) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.recordingPaths = append(route.recordingPaths, path)
}

func (route *agySharedCommandRoute) recordingPathsSnapshot() []string {
	route.mu.Lock()
	defer route.mu.Unlock()
	return append([]string(nil), route.recordingPaths...)
}

// agySharedCommandRunner selects solely from the normalized provider WorkDir.
// Registration is closed before root.BuildProcess, so an invocation cannot
// mutate routing or select a sibling through mutable session data.
type agySharedCommandRunner struct {
	mu          sync.Mutex
	routes      map[string]*agySharedCommandRoute
	scopeRoutes map[string]*agySharedCommandRoute
	selectors   map[string]struct{}
	frozen      bool
	requests    []platformprocess.CommandRequest
	active      int
	maxActive   int
	callSignal  chan struct{}
}

func newAgySharedCommandRunner() *agySharedCommandRunner {
	return &agySharedCommandRunner{
		routes:      make(map[string]*agySharedCommandRoute),
		scopeRoutes: make(map[string]*agySharedCommandRoute),
		selectors:   make(map[string]struct{}),
		callSignal:  make(chan struct{}, 64),
	}
}

func (runner *agySharedCommandRunner) registerScope(scopeID string, route *agySharedCommandRoute) error {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || route == nil {
		return fmt.Errorf("AGY execution scope and route are required")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, exists := runner.scopeRoutes[scopeID]; exists {
		return fmt.Errorf("AGY execution scope %q is already registered", scopeID)
	}
	runner.scopeRoutes[scopeID] = route
	return nil
}

func (runner *agySharedCommandRunner) unregisterScope(scopeID string, route *agySharedCommandRoute) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if current := runner.scopeRoutes[strings.TrimSpace(scopeID)]; current == route {
		delete(runner.scopeRoutes, strings.TrimSpace(scopeID))
	}
}

func (runner *agySharedCommandRunner) register(
	selector, workDir string,
	result platformprocess.CommandResult,
) (*agySharedCommandRoute, error) {
	return runner.registerOutcomes(selector, workDir, agySharedCommandOutcome{result: result})
}

func (runner *agySharedCommandRunner) registerOutcomes(
	selector, workDir string,
	outcomes ...agySharedCommandOutcome,
) (*agySharedCommandRoute, error) {
	selector = strings.TrimSpace(selector)
	normalized, err := normalizeAgyRoutePath(workDir)
	if err != nil {
		return nil, err
	}
	if selector == "" {
		return nil, fmt.Errorf("AGY route selector is required")
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.frozen {
		return nil, fmt.Errorf("AGY route table is frozen")
	}
	if _, exists := runner.routes[normalized]; exists {
		return nil, fmt.Errorf("AGY normalized WorkDir route is already registered")
	}
	if _, exists := runner.selectors[selector]; exists {
		return nil, fmt.Errorf("AGY route selector is already registered")
	}
	if len(outcomes) == 0 {
		return nil, fmt.Errorf("AGY route %q has no command outcome", selector)
	}

	route := &agySharedCommandRoute{
		selector: selector,
		workDir:  filepath.Clean(workDir),
		outcomes: cloneAgyCommandOutcomes(outcomes),
	}
	runner.routes[normalized] = route
	runner.selectors[selector] = struct{}{}
	return route, nil
}

func (runner *agySharedCommandRunner) freeze() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.frozen = true
}

func (runner *agySharedCommandRunner) routeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.routes)
}

func (runner *agySharedCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *agySharedCommandRunner) clear() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.frozen {
		return fmt.Errorf("AGY route table was not frozen")
	}
	if runner.active != 0 {
		return fmt.Errorf("AGY route table has %d active calls", runner.active)
	}
	runner.routes = nil
	runner.scopeRoutes = nil
	runner.selectors = nil
	runner.requests = nil
	runner.callSignal = nil
	return nil
}

func (runner *agySharedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	normalized, err := normalizeAgyRoutePath(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("AGY invocation route rejected")
	}

	runner.mu.Lock()
	if !runner.frozen {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route table is not frozen")
	}
	route := runner.scopeRoutes[strings.TrimSpace(request.ExecutionScopeID)]
	if route == nil {
		route = runner.routes[normalized]
	}
	ok := route != nil
	if !ok {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY invocation has no frozen route")
	}
	if request.Command != "agy" {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route received an unexpected command")
	}
	if err := ctx.Err(); err != nil {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, err
	}
	runner.requests = append(runner.requests, cloneAgyCommandRequest(request))
	runner.active++
	if runner.active > runner.maxActive {
		runner.maxActive = runner.active
	}
	callSignal := runner.callSignal
	runner.mu.Unlock()

	select {
	case callSignal <- struct{}{}:
	default:
	}
	defer func() {
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
	}()
	return route.record(ctx, request)
}

func (runner *agySharedCommandRunner) activeCallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active
}

func (runner *agySharedCommandRunner) maxActiveCallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.maxActive
}

func (runner *agySharedCommandRunner) waitForCallCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.NewTimer(agySharedInvocationTimeout)
	defer deadline.Stop()
	for {
		runner.mu.Lock()
		got := len(runner.requests)
		signal := runner.callSignal
		runner.mu.Unlock()
		if got >= want {
			return
		}
		select {
		case <-signal:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d AGY calls; got %d", want, got)
		}
	}
}

func normalizeAgyRoutePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("AGY route WorkDir is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize AGY route WorkDir: %w", err)
	}
	clean := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		clean = strings.ToLower(clean)
	}
	return clean, nil
}

func cloneAgyCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneAgyCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func cloneAgyCommandOutcomes(outcomes []agySharedCommandOutcome) []agySharedCommandOutcome {
	cloned := make([]agySharedCommandOutcome, len(outcomes))
	for index, outcome := range outcomes {
		cloned[index] = outcome
		cloned[index].result = cloneAgyCommandResult(outcome.result)
	}
	return cloned
}
