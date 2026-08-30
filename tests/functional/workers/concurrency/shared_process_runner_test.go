package root_composition_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func scaffoldConcurrencyFactory(t *testing.T, name string, capacity, maxRetries int) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name":      name,
		"resources": []map[string]any{{"id": "agent-slot", "name": "agent-slot", "capacity": capacity}},
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"type":      "MODEL_WORKSTATION",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		}},
	})
	return dir
}

func concurrencyWorkstationConfig(maxRetries int) string {
	limits := ""
	if maxRetries > 0 {
		limits = fmt.Sprintf("limits:\n  maxRetries: %d\n", maxRetries)
	}
	return "---\n" +
		"type: MODEL_WORKSTATION\n" +
		limits +
		"---\n" +
		"Return the input marker exactly once: {{ (index .Inputs 0).Payload }}\n"
}

type concurrencyStartedCall struct {
	index   int
	request platformprocess.CommandRequest
}

type concurrencyRunnerGate struct {
	channel chan struct{}
	once    sync.Once
}

type concurrencyScenarioRunner struct {
	behavior   concurrencyRunnerBehavior
	marker     string
	failMarker string

	mu          sync.Mutex
	next        int
	calls       []platformprocess.CommandRequest
	gates       map[int]*concurrencyRunnerGate
	releasedAll bool

	started            chan concurrencyStartedCall
	canceled           chan concurrencyStartedCall
	active             atomic.Int32
	peak               atomic.Int32
	finished           atomic.Int32
	canceledCountValue atomic.Int32
}

func newConcurrencyScenarioRunner(behavior concurrencyRunnerBehavior, marker, failMarker string) *concurrencyScenarioRunner {
	return &concurrencyScenarioRunner{
		behavior:   behavior,
		marker:     marker,
		failMarker: failMarker,
		gates:      make(map[int]*concurrencyRunnerGate),
		started:    make(chan concurrencyStartedCall, 128),
		canceled:   make(chan concurrencyStartedCall, 128),
	}
}

func (runner *concurrencyScenarioRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	updateConcurrencyPeak(&runner.peak, runner.active.Load())
	call := runner.record(request)
	defer func() {
		runner.active.Add(-1)
		runner.finished.Add(1)
	}()

	switch runner.behavior {
	case concurrencyRunnerSuccess:
		return runner.successResult(request), nil
	case concurrencyRunnerHold, concurrencyRunnerFailureHold:
		if err := runner.waitGate(ctx, call.index); err != nil {
			runner.canceledCountValue.Add(1)
			runner.canceled <- call
			return platformprocess.CommandResult{}, err
		}
		if runner.behavior == concurrencyRunnerFailureHold && commandRequestContains(request, runner.failMarker) {
			return runner.failureResult(), nil
		}
		return runner.successResult(request), nil
	case concurrencyRunnerTimeoutMarker:
		if commandRequestContains(request, runner.marker) {
			return platformprocess.CommandResult{ExitCode: 124, Stderr: []byte("request timed out")}, nil
		}
		return runner.successResult(request), nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unknown concurrency runner behavior %q", runner.behavior)
	}
}

func (runner *concurrencyScenarioRunner) record(request platformprocess.CommandRequest) concurrencyStartedCall {
	runner.mu.Lock()
	runner.next++
	call := concurrencyStartedCall{index: runner.next, request: cloneConcurrencyCommandRequest(request)}
	runner.calls = append(runner.calls, call.request)
	runner.gates[call.index] = &concurrencyRunnerGate{channel: make(chan struct{})}
	released := runner.releasedAll
	runner.mu.Unlock()
	runner.started <- call
	if released {
		runner.releaseCall(call.index)
	}
	return call
}

func (runner *concurrencyScenarioRunner) waitGate(ctx context.Context, index int) error {
	runner.mu.Lock()
	gate := runner.gates[index]
	released := runner.releasedAll
	runner.mu.Unlock()
	if released {
		return nil
	}
	select {
	case <-gate.channel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *concurrencyScenarioRunner) successResult(request platformprocess.CommandRequest) platformprocess.CommandResult {
	marker := concurrencyRequestMarker(request)
	if marker == "" {
		marker = runner.marker
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(marker + " output COMPLETE")}
}

func (runner *concurrencyScenarioRunner) failureResult() platformprocess.CommandResult {
	return platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: unexpected status 401 Unauthorized {\"type\":\"authentication_error\",\"message\":\"" + concurrencyFailureMessage + "\"}"),
	}
}

func (runner *concurrencyScenarioRunner) releaseCall(index int) {
	runner.mu.Lock()
	gate := runner.gates[index]
	runner.mu.Unlock()
	if gate != nil {
		gate.once.Do(func() { close(gate.channel) })
	}
}

func (runner *concurrencyScenarioRunner) releaseAll() {
	runner.mu.Lock()
	runner.releasedAll = true
	gates := make([]*concurrencyRunnerGate, 0, len(runner.gates))
	for _, gate := range runner.gates {
		gates = append(gates, gate)
	}
	runner.mu.Unlock()
	for _, gate := range gates {
		gate.once.Do(func() { close(gate.channel) })
	}
}

func (runner *concurrencyScenarioRunner) waitStarted(t testing.TB, timeout time.Duration) concurrencyStartedCall {
	t.Helper()
	select {
	case call := <-runner.started:
		return call
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for concurrency command start within %s", timeout)
		return concurrencyStartedCall{}
	}
}

func (runner *concurrencyScenarioRunner) waitStartedMarker(t testing.TB, marker string, timeout time.Duration) concurrencyStartedCall {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case call := <-runner.started:
			if commandRequestContains(call.request, marker) {
				return call
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for concurrency command marker %q", marker)
			return concurrencyStartedCall{}
		}
	}
}

func (runner *concurrencyScenarioRunner) waitCanceled(t testing.TB, timeout time.Duration) concurrencyStartedCall {
	t.Helper()
	select {
	case call := <-runner.canceled:
		return call
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for canceled concurrency command")
		return concurrencyStartedCall{}
	}
}

func (runner *concurrencyScenarioRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.calls)
}

func (runner *concurrencyScenarioRunner) callsForMarker(marker string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	count := 0
	for _, request := range runner.calls {
		if commandRequestContains(request, marker) {
			count++
		}
	}
	return count
}

func (runner *concurrencyScenarioRunner) activeCallCount() int {
	return int(runner.active.Load())
}

func (runner *concurrencyScenarioRunner) maxActive() int {
	return int(runner.peak.Load())
}

func (runner *concurrencyScenarioRunner) finishedCount() int {
	return int(runner.finished.Load())
}

func (runner *concurrencyScenarioRunner) canceledCount() int {
	return int(runner.canceledCountValue.Load())
}

func updateConcurrencyPeak(peak *atomic.Int32, current int32) {
	for {
		previous := peak.Load()
		if current <= previous || peak.CompareAndSwap(previous, current) {
			return
		}
	}
}

type concurrencyRoutedCall struct {
	selector string
	request  platformprocess.CommandRequest
}

type concurrencyCommandRouter struct {
	mu     sync.Mutex
	routes map[string]*concurrencyScenarioRunner
	calls  []concurrencyRoutedCall
	active atomic.Int32
	peak   atomic.Int32
}

func newConcurrencyCommandRouter() *concurrencyCommandRouter {
	return &concurrencyCommandRouter{routes: make(map[string]*concurrencyScenarioRunner)}
}

func (router *concurrencyCommandRouter) register(t *testing.T, dir string, runner *concurrencyScenarioRunner) {
	t.Helper()
	selector := filepath.Clean(strings.TrimSpace(dir))
	if selector == "" || selector == "." {
		t.Fatalf("empty concurrency Factory selector for %q", dir)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; exists {
		t.Fatalf("duplicate concurrency Factory selector %q", selector)
	}
	router.routes[selector] = runner
}

func (router *concurrencyCommandRouter) unregister(dir string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.routes, filepath.Clean(strings.TrimSpace(dir)))
}

func (router *concurrencyCommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	router.mu.Lock()
	runner, ok := router.routes[selector]
	if ok {
		router.calls = append(router.calls, concurrencyRoutedCall{selector: selector, request: cloneConcurrencyCommandRequest(request)})
	}
	router.mu.Unlock()
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown concurrency Factory selector %q; refusing cross-case route", request.WorkDir)
	}
	router.active.Add(1)
	updateConcurrencyPeak(&router.peak, router.active.Load())
	defer router.active.Add(-1)
	return runner.Run(ctx, request)
}

func (router *concurrencyCommandRouter) callsFor(dir string) []concurrencyRoutedCall {
	selector := filepath.Clean(strings.TrimSpace(dir))
	router.mu.Lock()
	defer router.mu.Unlock()
	var calls []concurrencyRoutedCall
	for _, call := range router.calls {
		if call.selector == selector {
			call.request = cloneConcurrencyCommandRequest(call.request)
			calls = append(calls, call)
		}
	}
	return calls
}

func (router *concurrencyCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

func (router *concurrencyCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *concurrencyCommandRouter) clearRoutes() {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes = nil
}

func (router *concurrencyCommandRouter) activeCount() int {
	return int(router.active.Load())
}

func (router *concurrencyCommandRouter) maxActive() int {
	return int(router.peak.Load())
}

func commandRequestContains(request platformprocess.CommandRequest, marker string) bool {
	if marker == "" {
		return false
	}
	if strings.Contains(string(request.Stdin), marker) {
		return true
	}
	for _, arg := range request.Args {
		if strings.Contains(arg, marker) {
			return true
		}
	}
	return false
}

var concurrencyMarkerPattern = regexp.MustCompile(`\bcc[0-9]+(?:-[A-Za-z0-9]+)*\b`)

func concurrencyRequestMarker(request platformprocess.CommandRequest) string {
	if match := concurrencyMarkerPattern.FindString(string(request.Stdin)); match != "" {
		return match
	}
	for _, arg := range request.Args {
		if match := concurrencyMarkerPattern.FindString(arg); match != "" {
			return match
		}
	}
	return ""
}

func cloneConcurrencyCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

type concurrencyIdentityGenerator struct {
	sessions      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *concurrencyIdentityGenerator) nextSessionID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *concurrencyIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("concurrency-shared-response-event-%d", generator.responseEvent.Add(1))
}
