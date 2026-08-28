package base

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := CloseGlobalFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared provider fixture: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type baseCaptureCommandRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *baseCaptureCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func (runner *baseCaptureCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

type baseFailureCommandRunner struct{ message string }

func (runner baseFailureCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stderr: []byte(runner.message), ExitCode: 1}, nil
}

type baseTimeoutThenSuccessCommandRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *baseTimeoutThenSuccessCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()
	if call == 1 {
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (runner *baseTimeoutThenSuccessCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

type baseCanceledCommandRunner struct{}

func (baseCanceledCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, context.Canceled
}

func assertBaseSessionPlaces(t testing.TB, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertBaseDispatchOutput(t testing.TB, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func assertBaseDispatchErrorContains(t testing.TB, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Error == nil || !strings.Contains(*payload.Error, want) {
			t.Fatalf("dispatch error = %#v, want substring %q", payload.Error, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func TestProvidersSharedProcessTopology(t *testing.T) {
	fixture := FixtureFor(t)
	fixture.AssertTopology(t)

	firstDir := testutilCopySharedFixture(t, "script_executor_dir")
	secondDir := testutilCopySharedFixture(t, "script_executor_dir")
	testutil.WriteSeedFile(t, firstDir, "task", []byte("first shared payload"))
	testutil.WriteSeedFile(t, secondDir, "task", []byte("second shared payload"))
	first := fixture.OpenScenario(t, firstDir, firstDir, support.NewStaticSuccessCommandRunner("first-shared-output"))
	second := fixture.OpenScenario(t, secondDir, secondDir, support.NewStaticSuccessCommandRunner("second-shared-output"))
	if got := fixture.router.routeCount(); got != 2 {
		t.Fatalf("shared provider active routes = %d, want two", got)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		first.WaitForTerminal(t, FixtureTimeout)
	}()
	go func() {
		defer wait.Done()
		second.WaitForTerminal(t, FixtureTimeout)
	}()
	wait.Wait()
	firstWork := first.ListWork(t)
	secondWork := second.ListWork(t)
	assertBaseSessionPlaces(t, firstWork, map[string]int{"task:done": 1, "task:init": 0})
	assertBaseSessionPlaces(t, secondWork, map[string]int{"task:done": 1, "task:init": 0})
	assertBaseDispatchOutput(t, first.FactoryEvents(t), "first-shared-output")
	assertBaseDispatchOutput(t, second.FactoryEvents(t), "second-shared-output")
	first.Stop(t)
	second.Stop(t)
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("shared provider routes after cleanup = %d, want zero", got)
	}
	fixture.AssertSessionTopology(t)
}

func TestProvidersSharedProcessRoutes(t *testing.T) {
	fixture := FixtureFor(t)
	fixture.AssertTopology(t)
	baselineCalls := fixture.router.callCount()

	workDir := filepath.Join(fixture.rootDir, "route-test-workdir")
	routeSelector := fmt.Sprintf("providers-shared-route-test-%d", sharedProviderRouteSequence.Add(1))
	runner := support.NewStaticSuccessCommandRunner("registered-route-output")
	if err := fixture.router.register(routeSelector, workDir, runner); err != nil {
		t.Fatalf("register shared provider test route: %v", err)
	}
	defer func() {
		if err := fixture.router.unregister(routeSelector); err != nil {
			t.Errorf("unregister shared provider test route: %v", err)
		}
	}()

	if err := fixture.router.register(routeSelector, workDir, support.NewStaticSuccessCommandRunner("duplicate-output")); err == nil {
		t.Fatal("duplicate shared provider route registration succeeded")
	}
	if got := fixture.router.routeCount(); got != 1 {
		t.Fatalf("shared provider route count after duplicate registration = %d, want one", got)
	}

	result, err := fixture.router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo", WorkDir: workDir, Stdin: []byte("registered route input"),
	})
	if err != nil {
		t.Fatalf("registered shared provider route: %v", err)
	}
	if string(result.Stdout) != "registered-route-output" {
		t.Fatalf("registered shared provider route output = %q, want registered output", result.Stdout)
	}
	if got := fixture.router.callCount(); got != baselineCalls+1 {
		t.Fatalf("shared provider route calls after registered route = %d, want %d", got, baselineCalls+1)
	}

	unknownWorkDir := filepath.Join(fixture.rootDir, "unknown-route-workdir")
	_, err = fixture.router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo", WorkDir: unknownWorkDir, Stdin: []byte("must not cross a route"),
	})
	if err == nil || !strings.Contains(err.Error(), "no provider route matched WorkDir") {
		t.Fatalf("unknown shared provider route error = %v, want explicit route failure", err)
	}
	if got := fixture.router.callCount(); got != baselineCalls+1 {
		t.Fatalf("shared provider route calls after unknown route = %d, want %d", got, baselineCalls+1)
	}
}

func TestProvidersSharedProcessAdverseRecovery(t *testing.T) {
	fixture := FixtureFor(t)

	t.Run("invalid_template", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		writeBaseFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, "---\ntype: MODEL_WORKSTATION\n---\n{{")
		testutil.WriteSeedFile(t, dir, "task", []byte("invalid-template-payload"))
		runner := &baseCaptureCommandRunner{}
		scenario, listed := RunFactory(t, dir, dir, runner, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		if got := runner.CallCount(); got != 0 {
			t.Fatalf("invalid-template provider calls = %d, want zero", got)
		}
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "prompt render failed")
		scenario.Stop(t)
	})

	t.Run("dependency_failure", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("dependency-failure-payload"))
		scenario, listed := RunFactory(t, dir, dir, baseFailureCommandRunner{"adverse dependency failure"}, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "adverse dependency failure")
		scenario.Stop(t)
	})

	t.Run("timeout", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		support.WriteWorkstationConfig(t, dir, "run-script", "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n")
		testutil.WriteSeedFile(t, dir, "task", []byte("timeout-payload"))
		runner := &baseTimeoutThenSuccessCommandRunner{}
		scenario, listed := RunFactory(t, dir, dir, runner, 10*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		if got := runner.CallCount(); got < 2 {
			t.Fatalf("timeout recovery provider calls = %d, want at least two", got)
		}
		scenario.Stop(t)
	})

	t.Run("cancellation", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("cancellation-payload"))
		scenario, listed := RunFactory(t, dir, dir, baseCanceledCommandRunner{}, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "execution cancelled: context canceled")
		scenario.Stop(t)
	})

	t.Run("unknown_route", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("unknown-route-payload"))
		scenario := fixture.OpenScenario(t, dir, "", nil)
		scenario.WaitForTerminal(t, 5*time.Second)
		listed := scenario.ListWork(t)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "script command execution failed")
		scenario.Stop(t)
	})

	t.Run("known_good_after_adverse_cases", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("known-good-payload"))
		scenario, listed := RunFactory(t, dir, dir, support.NewStaticSuccessCommandRunner("known-good-output"), 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertBaseDispatchOutput(t, scenario.FactoryEvents(t), "known-good-output")
		scenario.Stop(t)
	})

	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("shared provider routes after adverse recovery = %d, want zero", got)
	}
}

func testutilCopySharedFixture(t *testing.T, name string) string {
	t.Helper()
	source := support.LegacyFixtureDir(t, name)
	return testutil.CopyFixtureDir(t, source)
}

func writeBaseFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()
	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
