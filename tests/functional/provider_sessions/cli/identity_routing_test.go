package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workerSessionRoutePrefix = "worker-session-route="

// providerCommandRouteRunner keeps each provider result attached to the
// immutable Work name rendered into the provider prompt. The platform command
// edge intentionally carries no private execution metadata, so the
// package-local workstation fixture exposes the already customer-visible Work
// identity as a deterministic request marker. Calls may arrive concurrently
// and are never assigned a result by arrival order.
type providerCommandRouteRunner struct {
	mu           sync.Mutex
	routes       map[string]platformprocess.CommandResult
	dynamicGates map[string]*providerCommandRouteGate
	requests     []platformprocess.CommandRequest
	calls        chan struct{}
	active       int
}

type providerCommandRouteGate struct {
	mu       sync.Mutex
	channel  chan struct{}
	released bool
}

func newProviderCommandRouteGate() *providerCommandRouteGate {
	return &providerCommandRouteGate{channel: make(chan struct{})}
}

func (gate *providerCommandRouteGate) reset() {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if !gate.released {
		close(gate.channel)
	}
	gate.channel = make(chan struct{})
	gate.released = false
}

func (gate *providerCommandRouteGate) release() {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.released {
		return
	}
	close(gate.channel)
	gate.released = true
}

func (gate *providerCommandRouteGate) wait(ctx context.Context) error {
	gate.mu.Lock()
	channel := gate.channel
	gate.mu.Unlock()
	select {
	case <-channel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newProviderCommandRouteRunnerWithDynamicGates(
	routes map[string]platformprocess.CommandResult,
	gates map[string]*providerCommandRouteGate,
) *providerCommandRouteRunner {
	cloned := make(map[string]platformprocess.CommandResult, len(routes))
	for key, result := range routes {
		cloned[key] = cloneCommandResult(result)
	}
	clonedGates := make(map[string]*providerCommandRouteGate, len(gates))
	for key, gate := range gates {
		clonedGates[key] = gate
	}
	return &providerCommandRouteRunner{
		routes: cloned, dynamicGates: clonedGates,
		calls: make(chan struct{}, len(routes)+8),
	}
}

func (runner *providerCommandRouteRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	key := providerCommandRouteKey(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneCommandRequest(request))
	result, ok := runner.routes[key]
	dynamicGate := runner.dynamicGates[key]
	if ok {
		runner.active++
	}
	runner.mu.Unlock()
	if ok {
		defer func() {
			runner.mu.Lock()
			runner.active--
			runner.mu.Unlock()
		}()
	}
	select {
	case runner.calls <- struct{}{}:
	default:
	}
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"provider fixture route missing for immutable Work marker %q (stdin=%q)",
			key,
			string(request.Stdin),
		)
	}
	if dynamicGate != nil {
		if err := dynamicGate.wait(ctx); err != nil {
			return platformprocess.CommandResult{}, err
		}
	}
	return cloneCommandResult(result), nil
}

func (runner *providerCommandRouteRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *providerCommandRouteRunner) WaitForCalls(ctx context.Context, want int) error {
	for {
		if runner.CallCount() >= want {
			return nil
		}
		select {
		case <-runner.calls:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *providerCommandRouteRunner) RequestsSince(start int) []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if start > len(runner.requests) {
		start = len(runner.requests)
	}
	requests := make([]platformprocess.CommandRequest, len(runner.requests)-start)
	for index, request := range runner.requests[start:] {
		requests[index] = cloneCommandRequest(request)
	}
	return requests
}

func (runner *providerCommandRouteRunner) ActiveCallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active
}

func assertProviderCommandRoutesSince(t *testing.T, runner *providerCommandRouteRunner, start int, want map[string]struct{}) {
	t.Helper()
	requests := runner.RequestsSince(start)
	if len(requests) != len(want) {
		t.Fatalf("provider command route count = %d, want %d: %#v", len(requests), len(want), requests)
	}
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		key := providerCommandRouteKey(request)
		if _, ok := want[key]; !ok {
			t.Fatalf("provider command used unexpected immutable Work route %q; want %#v", key, want)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("provider command reused immutable Work route %q", key)
		}
		seen[key] = struct{}{}
	}
	for key := range want {
		if _, ok := seen[key]; !ok {
			t.Fatalf("provider command omitted immutable Work route %q", key)
		}
	}
}

func providerCommandRouteKey(request platformprocess.CommandRequest) string {
	for _, line := range strings.Split(string(request.Stdin), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, workerSessionRoutePrefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, workerSessionRoutePrefix))
		}
	}
	return ""
}

func cloneCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

// writeWorkerSessionRouteWorkstation makes the immutable Work name available
// at the injected provider-command edge without changing shared fixtures.
func writeWorkerSessionRouteWorkstation(t *testing.T, factoryDir string) {
	t.Helper()
	support.WriteWorkstationConfig(t, factoryDir, "process", "---\ntype: MODEL_WORKSTATION\n---\nworker-session-route={{ (index .Inputs 0).Name }}\n")
}

func openExplicitWorkerSession(t *testing.T, baseURL, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("open Factory Session response = %#v, want identity", opened)
	}
	if opened.Session.IsDefault || opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("open Factory Session returned default identity = %#v, want explicit non-default session", opened.Session)
	}
	return opened.Session.Id
}

func assertFactorySessionAbsent(t *testing.T, baseURL, sessionID, factoryDir string) {
	t.Helper()
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET closed Factory Session %q: %v", sessionID, err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET closed Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions"
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, endpoint)
	for _, session := range listed.Sessions {
		if session.Id == sessionID && (session.FolderPath == factoryDir || session.FactoryDir == factoryDir) {
			t.Fatalf("closed Factory Session %q remained addressable for folder %q in live list: %#v", sessionID, factoryDir, listed)
		}
	}
}

var _ platformprocess.CommandRunner = (*providerCommandRouteRunner)(nil)
