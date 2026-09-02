package cli_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
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
	mu            sync.Mutex
	definitions   map[string]providerCommandRoute
	routes        map[string]providerCommandRoute
	activeByRoute map[string]int
	requests      []platformprocess.CommandRequest
	calls         chan struct{}
	active        int
}

type providerCommandRoute struct {
	result platformprocess.CommandResult
	gate   *providerCommandRouteGate
}

// providerCommandRouteRegistration owns one active route. Close is idempotent
// so case cleanup remains deterministic even when a test reports an earlier
// assertion failure while its cleanup stack is unwinding.
type providerCommandRouteRegistration struct {
	runner *providerCommandRouteRunner
	key    string
	once   sync.Once
	err    error
}

func (registration *providerCommandRouteRegistration) Close() error {
	if registration == nil {
		return nil
	}
	registration.once.Do(func() {
		registration.err = registration.runner.unregisterRoute(registration.key)
	})
	return registration.err
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
	definitions := make(map[string]providerCommandRoute, len(routes))
	for key, result := range routes {
		definitions[key] = providerCommandRoute{result: cloneCommandResult(result)}
	}
	for key, gate := range gates {
		definition := definitions[key]
		definition.gate = gate
		definitions[key] = definition
	}
	return &providerCommandRouteRunner{
		definitions:   definitions,
		routes:        make(map[string]providerCommandRoute),
		activeByRoute: make(map[string]int),
		calls:         make(chan struct{}, len(routes)+8),
	}
}

func (runner *providerCommandRouteRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	key := providerCommandRouteKey(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneCommandRequest(request))
	route, ok := runner.routes[key]
	if ok {
		runner.active++
		runner.activeByRoute[key]++
	}
	runner.mu.Unlock()
	if ok {
		defer func() {
			runner.mu.Lock()
			runner.active--
			runner.activeByRoute[key]--
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
	if route.gate != nil {
		if err := route.gate.wait(ctx); err != nil {
			return platformprocess.CommandResult{}, err
		}
	}
	return cloneCommandResult(route.result), nil
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

// registerRoute models the case-owned registration boundary used by the
// controlled provider fixture. Registration is deliberately fail-closed so a
// duplicate immutable Work marker cannot silently replace an existing result.
func (runner *providerCommandRouteRunner) registerRoute(key string) (*providerCommandRouteRegistration, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("provider fixture route key is empty")
	}
	definition, defined := runner.definitions[key]
	if !defined {
		return nil, fmt.Errorf("provider fixture route %q is not defined", key)
	}
	if _, registered := runner.routes[key]; registered {
		return nil, fmt.Errorf("provider fixture route %q is already registered", key)
	}
	runner.routes[key] = providerCommandRoute{
		result: cloneCommandResult(definition.result),
		gate:   definition.gate,
	}
	return &providerCommandRouteRegistration{runner: runner, key: key}, nil
}

func (runner *providerCommandRouteRunner) unregisterRoute(key string) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, registered := runner.routes[key]; !registered {
		return fmt.Errorf("provider fixture route %q is not registered", key)
	}
	if active := runner.activeByRoute[key]; active != 0 {
		return fmt.Errorf("provider fixture route %q has %d active calls", key, active)
	}
	delete(runner.routes, key)
	delete(runner.activeByRoute, key)
	return nil
}

func (runner *providerCommandRouteRunner) close() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.active != 0 {
		return fmt.Errorf("provider fixture route runner has %d active calls", runner.active)
	}
	if len(runner.routes) != 0 {
		keys := make([]string, 0, len(runner.routes))
		for key := range runner.routes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return fmt.Errorf("provider fixture routes remain registered: %s", strings.Join(keys, ", "))
	}
	runner.definitions = nil
	runner.activeByRoute = nil
	return nil
}

func (runner *providerCommandRouteRunner) routeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.routes)
}

func (runner *providerCommandRouteRunner) activeRouteKeys() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	keys := make([]string, 0, len(runner.routes))
	for key := range runner.routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertNoActiveProviderCommandRoutes(t *testing.T, runner *providerCommandRouteRunner, owner string) {
	t.Helper()
	if keys := runner.activeRouteKeys(); len(keys) != 0 {
		t.Fatalf("Provider Sessions CLI routes remain after %s: %s", owner, strings.Join(keys, ", "))
	}
}

func assertProviderCommandRoutesSince(t *testing.T, runner *providerCommandRouteRunner, start int, want map[string]struct{}) {
	t.Helper()
	requests := runner.RequestsSince(start)
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		key := providerCommandRouteKey(request)
		if _, ok := want[key]; !ok {
			continue
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

func assertFactorySessionFolderAbsent(t *testing.T, baseURL, factoryDir string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions",
	)
	for _, session := range listed.Sessions {
		if session.FolderPath == factoryDir || session.FactoryDir == factoryDir {
			t.Fatalf("Factory Session %q remained for folder %q after failed setup: %#v", session.Id, factoryDir, listed)
		}
	}
}

var _ platformprocess.CommandRunner = (*providerCommandRouteRunner)(nil)
