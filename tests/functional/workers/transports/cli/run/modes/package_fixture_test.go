package modes_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// This deadline is only a bounded failure guard around the deterministic
// invocation-done, provider-started, cancellation, and Process.Close
// signals below. Those channels synchronize the behavior; the deadline is
// needed only to fail a genuinely hung production Execute/Close lifecycle,
// which an edge mock cannot prove or replace.
const modesProcessStopTimeout = 30 * time.Second

type modesRouteBehavior string

const (
	modesRouteSuccess modesRouteBehavior = "success"
	modesRouteFailure modesRouteBehavior = "failure"
	modesRouteBlock   modesRouteBehavior = "block"
	modesRoutePartial modesRouteBehavior = "partial-failure"
)

const partialProviderStdout = `{"type":"thread.started","thread_id":"c11-partial"}
{"type":"item.started","item":{"id":"c11-partial-item","type":"agent_message"}}
`

// modesInvocationSpec describes only the mutable inputs and provider outcome
// for one public CLI invocation. The application graph remains package-owned.
type modesInvocationSpec struct {
	globalArgs     []string
	runArgs        []string
	prompt         string
	stdin          string
	result         string
	emptyResult    bool
	stdinSignature bool
	behavior       modesRouteBehavior
	includePrompt  bool
	context        context.Context
}

type modesInvocationResources struct {
	id          string
	routeID     string
	workingRoot string
	homeDir     string
	factoryPath string
}

type modesInvocationResult struct {
	stdout        string
	stderr        string
	err           error
	providerCalls int
	resources     modesInvocationResources
	requests      []platformprocess.CommandRequest
}

type modesInvocationHandle struct {
	fixture   *modesPackageFixture
	route     *modesCommandRoute
	inputs    *support.CapturedInputs
	resources modesInvocationResources
	result    modesInvocationResult
	done      chan modesInvocationResult
	cancel    context.CancelFunc

	finishOnce sync.Once
	finished   atomic.Bool
}

// modesPackageFixture substitutes only the ProviderCommandRunner edge for a
// single root-built process. Cleanup assertions therefore cover the actual
// process, invocation handles, registered edge routes, active provider calls,
// and temporary roots. Factory/Worker Session and response-stream closure,
// plus subprocess/listener/port lifecycle, remain owned by their public and
// built-binary integration gates; this fixture does not fabricate counters for
// those resources.
type modesPackageFixture struct {
	process     support.ApplicationProcess
	router      *modesCommandRouter
	rootDir     string
	factoryPath string

	processBuilds  atomic.Int32
	nextInvocation atomic.Uint64

	mu                sync.Mutex
	activeInvocations map[string]struct{}
	closeOnce         sync.Once
	closeErr          error
}

var modesPackageFixtureState struct {
	sync.Once
	fixture *modesPackageFixture
	err     error
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	closeModesPackageFixture()
	if modesPackageFixtureState.fixture != nil {
		fixture := modesPackageFixtureState.fixture
		if fixture.closeErr != nil {
			fmt.Fprintf(os.Stderr, "modes package fixture close error: %v\n", fixture.closeErr)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func modesFixture(t testing.TB) *modesPackageFixture {
	t.Helper()
	modesPackageFixtureState.Do(func() {
		modesPackageFixtureState.fixture, modesPackageFixtureState.err = newModesPackageFixture()
	})
	if modesPackageFixtureState.err != nil {
		t.Fatalf("create modes package fixture: %v", modesPackageFixtureState.err)
	}
	return modesPackageFixtureState.fixture
}

func newModesPackageFixture() (*modesPackageFixture, error) {
	rootDir, err := os.MkdirTemp("", "c11-modes-package-")
	if err != nil {
		return nil, err
	}
	factoryPath, err := writeModesFactory(rootDir)
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, err
	}
	router := newModesCommandRouter()
	process, err := support.BuildProcessWithContext(
		context.Background(),
		serviceedges.Edges{ProviderCommandRunner: router},
	)
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, err
	}
	fixture := &modesPackageFixture{
		process:           process,
		router:            router,
		rootDir:           rootDir,
		factoryPath:       factoryPath,
		activeInvocations: make(map[string]struct{}),
	}
	fixture.processBuilds.Store(1)
	return fixture, nil
}

func closeModesPackageFixture() {
	if modesPackageFixtureState.fixture == nil {
		return
	}
	fixture := modesPackageFixtureState.fixture
	fixture.closeOnce.Do(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), modesProcessStopTimeout)
		defer cancel()
		fixture.closeErr = fixture.close(closeCtx)
	})
}

func (fixture *modesPackageFixture) close(ctx context.Context) error {
	var errs []error
	if fixture.process != nil {
		if err := fixture.process.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close process: %w", err))
		}
	}

	fixture.mu.Lock()
	if len(fixture.activeInvocations) != 0 {
		errs = append(errs, fmt.Errorf("active invocation handles remain: %d", len(fixture.activeInvocations)))
	}
	fixture.mu.Unlock()
	if active := fixture.router.ActiveCallCount(); active != 0 {
		errs = append(errs, fmt.Errorf("active provider calls remain: %d", active))
	}
	if routes := fixture.router.RouteCount(); routes != 0 {
		errs = append(errs, fmt.Errorf("provider routes remain: %d", routes))
	}
	if fixture.processBuilds.Load() != 1 {
		errs = append(errs, fmt.Errorf("process builds=%d, want 1", fixture.processBuilds.Load()))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
	}

	fmt.Fprintf(
		os.Stderr,
		"modes package topology: builds=%d active_invocations=%d provider_routes=%d provider_calls=%d active_provider_calls=%d root_removed=%t\n",
		fixture.processBuilds.Load(), len(fixture.activeInvocations), fixture.router.RouteCount(),
		fixture.router.CallCount(), fixture.router.ActiveCallCount(), !pathExists(fixture.rootDir),
	)
	return errors.Join(errs...)
}

func (fixture *modesPackageFixture) execute(t testing.TB, spec modesInvocationSpec) modesInvocationResult {
	t.Helper()
	handle := fixture.start(t, spec)
	return handle.wait(t)
}

func (fixture *modesPackageFixture) start(t testing.TB, spec modesInvocationSpec) *modesInvocationHandle {
	t.Helper()
	invocationNumber := fixture.nextInvocation.Add(1)
	id := fmt.Sprintf("modes-invocation-%d", invocationNumber)
	workingRoot := filepath.Join(fixture.rootDir, "invocations", id)
	homeDir := filepath.Join(workingRoot, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create invocation roots: %v", err)
	}
	invocationFactoryPath := filepath.Join(workingRoot, "factory", "factory.json")
	if err := copyModesFactory(fixture.factoryPath, invocationFactoryPath, spec.emptyResult, spec.stdinSignature); err != nil {
		t.Fatalf("copy invocation Factory: %v", err)
	}
	resources := modesInvocationResources{
		id:          id,
		routeID:     id + "-route",
		workingRoot: workingRoot,
		homeDir:     homeDir,
		factoryPath: invocationFactoryPath,
	}
	route := newModesCommandRoute(resources.routeID, spec)
	if err := fixture.router.Register(workingRoot, route); err != nil {
		t.Fatalf("register provider route: %v", err)
	}
	fixture.openInvocation(resources)

	args := []string{"you"}
	args = append(args, spec.globalArgs...)
	args = append(args, "run", "--factory", invocationFactoryPath, "--no-record")
	args = append(args, spec.runArgs...)
	if spec.includePrompt {
		args = append(args, spec.prompt)
	}
	baseContext := spec.context
	if baseContext == nil {
		baseContext = t.Context()
	}
	ctx, cancel := context.WithCancel(baseContext)
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Args = append([]string(nil), args...)
	inputs.Input.Env = modesInvocationEnvironment(homeDir)
	inputs.Input.Stdin = strings.NewReader(spec.stdin)
	inputs.Input.WorkingDirectory = workingRoot

	handle := &modesInvocationHandle{
		fixture:   fixture,
		route:     route,
		inputs:    inputs,
		resources: resources,
		cancel:    cancel,
		done:      make(chan modesInvocationResult, 1),
	}
	go func() {
		err := fixture.process.Execute(inputs.Input)
		handle.done <- modesInvocationResult{
			stdout:        inputs.Stdout(),
			stderr:        inputs.Stderr(),
			err:           err,
			providerCalls: fixture.router.CallCountFor(route.id),
			resources:     resources,
			requests:      fixture.router.RequestsFor(route.id),
		}
	}()
	t.Cleanup(func() {
		if handle.isFinished() {
			return
		}
		handle.cancel()
		select {
		case result := <-handle.done:
			handle.result = result
			handle.finish()
		case <-time.After(modesProcessStopTimeout):
			t.Errorf("timed out waiting for invocation %s cleanup", resources.id)
		}
	})
	return handle
}

func (handle *modesInvocationHandle) wait(t testing.TB) modesInvocationResult {
	t.Helper()
	select {
	case result := <-handle.done:
		handle.result = result
		handle.finish()
		return result
	case <-time.After(modesProcessStopTimeout):
		handle.cancel()
		handle.finish()
		t.Fatalf("timed out waiting for invocation %s", handle.route.id)
		return modesInvocationResult{}
	}
}

func (handle *modesInvocationHandle) isFinished() bool {
	return handle.finished.Load()
}

func (handle *modesInvocationHandle) finish() {
	handle.finishOnce.Do(func() {
		handle.cancel()
		handle.fixture.router.Unregister(handle.inputs.Input.WorkingDirectory, handle.route)
		handle.fixture.closeInvocation(handle.resources)
		handle.finished.Store(true)
	})
}

func (fixture *modesPackageFixture) openInvocation(resources modesInvocationResources) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.activeInvocations[resources.id] = struct{}{}
}

func (fixture *modesPackageFixture) closeInvocation(resources modesInvocationResources) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if _, ok := fixture.activeInvocations[resources.id]; !ok {
		return
	}
	delete(fixture.activeInvocations, resources.id)
	_ = os.RemoveAll(resources.workingRoot)
}

func modesInvocationEnvironment(homeDir string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if ok && (strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE")) {
			continue
		}
		env = append(env, item)
	}
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func writeModesFactory(rootDir string) (string, error) {
	factoryDir := filepath.Join(rootDir, "factory")
	workerDir := filepath.Join(factoryDir, "workers", "worker-a")
	workstationDir := filepath.Join(factoryDir, "workstations", "process")
	for _, dir := range []string{workerDir, workstationDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	factoryConfig := map[string]any{
		"name": "c11-modes",
		"workTypes": []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "complete", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
				"handlingBehavior": []string{"DEFAULT"},
			},
		},
		"workers": []any{map[string]any{"name": "worker-a"}},
		"workstations": []any{
			map[string]any{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
				"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
			},
		},
	}
	factoryJSON, err := json.MarshalIndent(factoryConfig, "", "  ")
	if err != nil {
		return "", err
	}
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, append(factoryJSON, '\n'), 0o644); err != nil {
		return "", err
	}
	workerConfig := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerConfig), 0o644); err != nil {
		return "", err
	}
	workstationConfig := "---\ntype: MODEL_WORKSTATION\n---\n\nProcess the task deterministically.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(workstationConfig), 0o644); err != nil {
		return "", err
	}
	return factoryPath, nil
}

func copyModesFactory(sourcePath, targetPath string, decisionEnvelope, stdinSignature bool) error {
	sourceDir := filepath.Dir(sourcePath)
	targetDir := filepath.Dir(targetPath)
	for _, relative := range []string{
		"factory.json",
		filepath.Join("workers", "worker-a", "AGENTS.md"),
		filepath.Join("workstations", "process", "AGENTS.md"),
	} {
		data, err := os.ReadFile(filepath.Join(sourceDir, relative))
		if err != nil {
			return err
		}
		if relative == "factory.json" && decisionEnvelope {
			var factory map[string]any
			if err := json.Unmarshal(data, &factory); err != nil {
				return err
			}
			workstations, ok := factory["workstations"].([]any)
			if !ok {
				return errors.New("modes factory workstations are not an array")
			}
			for _, raw := range workstations {
				workstation, ok := raw.(map[string]any)
				if !ok {
					return errors.New("modes factory workstation is not an object")
				}
				workstation["outcomeFormat"] = "decision-envelope"
			}
			data, err = json.MarshalIndent(factory, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
		}
		if relative == "factory.json" && stdinSignature {
			var factory map[string]any
			if err := json.Unmarshal(data, &factory); err != nil {
				return err
			}
			factory["invocationSignature"] = map[string]any{
				"parameters": []any{map[string]any{
					"name":     "marker",
					"typeHint": "BOOLEAN_STRING",
					"bindings": []any{map[string]any{"kind": "STDIN"}},
				}},
			}
			data, err = json.MarshalIndent(factory, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
		}
		destination := filepath.Join(targetDir, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type modesRoutedCommand struct {
	routeID string
	request platformprocess.CommandRequest
}

type modesCommandRouter struct {
	mu     sync.Mutex
	routes map[string]*modesCommandRoute
	calls  []modesRoutedCommand
}

func newModesCommandRouter() *modesCommandRouter {
	return &modesCommandRouter{routes: make(map[string]*modesCommandRoute)}
}

func (router *modesCommandRouter) Register(workDir string, route *modesCommandRoute) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	key := filepath.Clean(workDir)
	if _, exists := router.routes[key]; exists {
		return fmt.Errorf("provider route already registered for %q", key)
	}
	router.routes[key] = route
	return nil
}

func (router *modesCommandRouter) Unregister(workDir string, route *modesCommandRoute) {
	router.mu.Lock()
	defer router.mu.Unlock()
	key := filepath.Clean(workDir)
	if current, ok := router.routes[key]; ok && current == route {
		delete(router.routes, key)
	}
}

func (router *modesCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return router.run(ctx, request, nil)
}

func (router *modesCommandRouter) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return router.run(ctx, request, observer)
}

func (router *modesCommandRouter) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	key := filepath.Clean(request.WorkDir)
	route := router.routes[key]
	if route != nil {
		router.calls = append(router.calls, modesRoutedCommand{
			routeID: route.id,
			request: cloneModesCommandRequest(request),
		})
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no provider route for work directory %q", request.WorkDir)
	}
	return route.run(ctx, request, observer)
}

func (router *modesCommandRouter) CallCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

func (router *modesCommandRouter) CallCountFor(routeID string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	count := 0
	for _, call := range router.calls {
		if call.routeID == routeID {
			count++
		}
	}
	return count
}

func (router *modesCommandRouter) RequestsFor(routeID string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, 0)
	for _, call := range router.calls {
		if call.routeID == routeID {
			requests = append(requests, cloneModesCommandRequest(call.request))
		}
	}
	return requests
}

func (router *modesCommandRouter) ActiveCallCount() int {
	router.mu.Lock()
	routes := make([]*modesCommandRoute, 0, len(router.routes))
	for _, route := range router.routes {
		routes = append(routes, route)
	}
	router.mu.Unlock()
	active := 0
	for _, route := range routes {
		active += int(route.active.Load())
	}
	return active
}

func (router *modesCommandRouter) RouteCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func cloneModesCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

type modesCommandRoute struct {
	id          string
	behavior    modesRouteBehavior
	result      string
	emptyResult bool
	exitCode    int
	stderr      string
	release     chan struct{}
	releaseOnce sync.Once
	started     chan struct{}
	startedOnce sync.Once
	active      atomic.Int32
}

func newModesCommandRoute(id string, spec modesInvocationSpec) *modesCommandRoute {
	behavior := spec.behavior
	if behavior == "" {
		behavior = modesRouteSuccess
	}
	result := spec.result
	if result == "" && !spec.emptyResult {
		result = wantPrimaryResult
	}
	return &modesCommandRoute{
		id:          id,
		behavior:    behavior,
		result:      result,
		emptyResult: spec.emptyResult,
		exitCode:    deterministicProviderFailureExit,
		stderr:      deterministicProviderFailureStderr,
		release:     make(chan struct{}),
		started:     make(chan struct{}),
	}
}

func (route *modesCommandRoute) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	route.active.Add(1)
	defer route.active.Add(-1)
	route.startedOnce.Do(func() { close(route.started) })
	if route.behavior == modesRouteBlock {
		select {
		case <-route.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{
				CancellationReason: platformprocess.CancellationReasonFromContext(ctx),
			}, ctx.Err()
		}
	}
	if route.behavior == modesRoutePartial && observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(partialProviderStdout))
	}
	if route.behavior == modesRouteFailure || route.behavior == modesRoutePartial {
		result := platformprocess.CommandResult{
			Stdout:   []byte(partialProviderStdout),
			Stderr:   []byte(route.stderr),
			ExitCode: route.exitCode,
		}
		if route.behavior == modesRouteFailure {
			result.Stdout = nil
		}
		return result, nil
	}
	stdout := modesCodexSuccessStdout(route.result, route.emptyResult)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, stdout)
	}
	return platformprocess.CommandResult{Stdout: stdout}, nil
}

func modesCodexSuccessStdout(result string, emptyResult bool) []byte {
	if !emptyResult {
		return support.CodexSuccessStdout(result)
	}
	return support.CodexSuccessStdout(`{"decision":"ACCEPTED","feedback":"accepted empty primary output","output":""}`)
}

func (route *modesCommandRoute) WaitStarted(t testing.TB) {
	t.Helper()
	select {
	case <-route.started:
	case <-time.After(modesProcessStopTimeout):
		t.Fatalf("timed out waiting for provider route %s", route.id)
	}
}

func (route *modesCommandRoute) Release() {
	route.releaseOnce.Do(func() { close(route.release) })
}

var _ interface {
	Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)
	RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
} = (*modesCommandRouter)(nil)
