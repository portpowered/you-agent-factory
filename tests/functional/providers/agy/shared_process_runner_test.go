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

type agySharedLifecycleEvent struct {
	elapsed time.Duration
	stage   string
	route   string
	scope   string
	detail  string
}

type agySharedLifecycleTraceBinding struct {
	trace   *agySharedLifecycleTrace
	scopeID string
}

// agySharedLifecycleTrace records only the route and Factory Session identities
// attached to one diagnostic scenario. It is deliberately observational: the
// existing aggregate rendezvous and public assertions remain unchanged while
// the trace distinguishes this scenario's provider calls from package peers.
type agySharedLifecycleTrace struct {
	mu          sync.Mutex
	started     time.Time
	events      []agySharedLifecycleEvent
	active      int
	maxActive   int
	bothEntered bool
}

func newAgySharedLifecycleTrace() *agySharedLifecycleTrace {
	return &agySharedLifecycleTrace{started: time.Now()}
}

func (trace *agySharedLifecycleTrace) record(
	stage, route, scope, detail string,
) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.events = append(trace.events, agySharedLifecycleEvent{
		elapsed: time.Since(trace.started),
		stage:   stage,
		route:   route,
		scope:   scope,
		detail:  detail,
	})
}

func (trace *agySharedLifecycleTrace) providerEntered(
	route, scope, requestScope string,
) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.active++
	if trace.active > trace.maxActive {
		trace.maxActive = trace.active
	}
	trace.events = append(trace.events, agySharedLifecycleEvent{
		elapsed: time.Since(trace.started),
		stage:   "provider-entered",
		route:   route,
		scope:   scope,
		detail: fmt.Sprintf(
			"active=%d maxActive=%d requestScope=%q",
			trace.active,
			trace.maxActive,
			requestScope,
		),
	})
	if trace.active == 2 && !trace.bothEntered {
		trace.bothEntered = true
		trace.events = append(trace.events, agySharedLifecycleEvent{
			elapsed: time.Since(trace.started),
			stage:   "both-entered",
			route:   route,
			scope:   scope,
			detail: fmt.Sprintf(
				"active=%d maxActive=%d requestScope=%q",
				trace.active,
				trace.maxActive,
				requestScope,
			),
		})
	}
}

func (trace *agySharedLifecycleTrace) providerReturned(
	route, scope, requestScope string,
	result platformprocess.CommandResult,
	err error,
) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if trace.active > 0 {
		trace.active--
	}
	outcome := fmt.Sprintf("active=%d exitCode=%d", trace.active, result.ExitCode)
	if err != nil {
		outcome = fmt.Sprintf("active=%d errorType=%T", trace.active, err)
	}
	outcome = fmt.Sprintf("%s requestScope=%q", outcome, requestScope)
	trace.events = append(trace.events, agySharedLifecycleEvent{
		elapsed: time.Since(trace.started),
		stage:   "provider-returned",
		route:   route,
		scope:   scope,
		detail:  outcome,
	})
}

func (trace *agySharedLifecycleTrace) activeCounts() (int, int) {
	if trace == nil {
		return 0, 0
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return trace.active, trace.maxActive
}

func (trace *agySharedLifecycleTrace) log(t testing.TB) {
	t.Helper()
	if trace == nil {
		return
	}
	trace.mu.Lock()
	events := append([]agySharedLifecycleEvent(nil), trace.events...)
	active := trace.active
	maxActive := trace.maxActive
	trace.mu.Unlock()
	t.Logf(
		"AGY lifecycle trace: events=%d traceActive=%d traceMaxActive=%d",
		len(events), active, maxActive,
	)
	for index, event := range events {
		t.Logf(
			"AGY lifecycle stage[%d]=%s elapsed=%s route=%q session=%q %s",
			index, event.stage, event.elapsed, event.route, event.scope, event.detail,
		)
	}
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

	mu            sync.Mutex
	requests      []platformprocess.CommandRequest
	outcomeCursor int
	traceBinding  agySharedLifecycleTraceBinding
}

func (route *agySharedCommandRoute) record(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	route.mu.Lock()
	index := route.outcomeCursor
	route.outcomeCursor++
	route.requests = append(route.requests, cloneAgyCommandRequest(request))
	outcome := route.outcome(index)
	binding := route.traceBinding
	route.mu.Unlock()
	if binding.trace != nil {
		binding.trace.record(
			"route-record-entered",
			route.selector,
			binding.scopeID,
			fmt.Sprintf("requestScope=%q", request.ExecutionScopeID),
		)
	}

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

func (route *agySharedCommandRoute) resetOutcomeSequence() {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.outcomeCursor = 0
}

func (route *agySharedCommandRoute) setRelease(release <-chan struct{}) {
	route.mu.Lock()
	defer route.mu.Unlock()
	for index := range route.outcomes {
		route.outcomes[index].release = release
	}
}

func (route *agySharedCommandRoute) bindLifecycleTrace(
	scopeID string,
	trace *agySharedLifecycleTrace,
) error {
	if trace == nil {
		return fmt.Errorf("AGY lifecycle trace is required")
	}
	route.mu.Lock()
	defer route.mu.Unlock()
	if route.traceBinding.trace != nil {
		return fmt.Errorf("AGY route %q already has a lifecycle trace", route.selector)
	}
	route.traceBinding = agySharedLifecycleTraceBinding{trace: trace, scopeID: scopeID}
	return nil
}

func (route *agySharedCommandRoute) setLifecycleTraceScope(
	scopeID string,
	trace *agySharedLifecycleTrace,
) error {
	route.mu.Lock()
	defer route.mu.Unlock()
	if route.traceBinding.trace != trace {
		return fmt.Errorf("AGY route %q has a different lifecycle trace", route.selector)
	}
	route.traceBinding.scopeID = scopeID
	return nil
}

func (route *agySharedCommandRoute) unbindLifecycleTrace(
	trace *agySharedLifecycleTrace,
) {
	route.mu.Lock()
	defer route.mu.Unlock()
	if route.traceBinding.trace == trace {
		route.traceBinding = agySharedLifecycleTraceBinding{}
	}
}

func (route *agySharedCommandRoute) lifecycleTraceBinding() agySharedLifecycleTraceBinding {
	route.mu.Lock()
	defer route.mu.Unlock()
	return route.traceBinding
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

func (runner *agySharedCommandRunner) registerScopeTrace(
	scopeID string,
	trace *agySharedLifecycleTrace,
) error {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || trace == nil {
		return fmt.Errorf("AGY execution scope and lifecycle trace are required")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, exists := runner.scopeRoutes[scopeID]; !exists {
		return fmt.Errorf("AGY execution scope %q is not registered", scopeID)
	}
	route := runner.scopeRoutes[scopeID]
	binding := route.lifecycleTraceBinding()
	if binding.trace == nil {
		return route.bindLifecycleTrace(scopeID, trace)
	}
	if binding.trace != trace {
		return fmt.Errorf("AGY route %q already has a different lifecycle trace", route.selector)
	}
	return route.setLifecycleTraceScope(scopeID, trace)
}

func (runner *agySharedCommandRunner) unregisterScope(scopeID string, route *agySharedCommandRoute) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if current := runner.scopeRoutes[strings.TrimSpace(scopeID)]; current == route {
		scopeID = strings.TrimSpace(scopeID)
		delete(runner.scopeRoutes, scopeID)
		route.unbindLifecycleTrace(route.lifecycleTraceBinding().trace)
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
	scopeID := strings.TrimSpace(request.ExecutionScopeID)
	route := runner.scopeRoutes[scopeID]
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
	binding := route.lifecycleTraceBinding()
	trace := binding.trace
	traceScopeID := binding.scopeID
	runner.requests = append(runner.requests, cloneAgyCommandRequest(request))
	runner.active++
	if runner.active > runner.maxActive {
		runner.maxActive = runner.active
	}
	callSignal := runner.callSignal
	if trace != nil {
		trace.providerEntered(route.selector, traceScopeID, request.ExecutionScopeID)
	}
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
	result, runErr := route.record(ctx, request)
	if trace != nil {
		trace.providerReturned(route.selector, traceScopeID, request.ExecutionScopeID, result, runErr)
	}
	return result, runErr
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
