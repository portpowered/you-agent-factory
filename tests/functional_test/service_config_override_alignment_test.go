package functional_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestServiceConfigOverrideAlignment_ProviderCommandRunner(t *testing.T) {
	t.Run("ServiceHarness", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider harness alignment"}`))

		runner := testutil.NewProviderCommandRunner(
			workers.CommandResult{Stdout: []byte("step one complete. COMPLETE")},
			workers.CommandResult{Stdout: []byte("step two complete. COMPLETE")},
		)
		harness := testutil.NewServiceTestHarness(t, dir,
			testutil.WithFullWorkerPoolAndScriptWrap(),
			testutil.WithProviderCommandRunner(runner),
		)

		harness.RunUntilComplete(t, 10*time.Second)

		harness.Assert().
			HasTokenInPlace("task:complete").
			HasNoTokenInPlace("task:failed")
		if got := runner.CallCount(); got != 2 {
			t.Fatalf("provider command runner calls = %d, want 2", got)
		}
	})

	t.Run("FunctionalHTTPServer", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider server alignment"}`))

		runner := testutil.NewProviderCommandRunner(
			workers.CommandResult{Stdout: []byte("step one complete. COMPLETE")},
			workers.CommandResult{Stdout: []byte("step two complete. COMPLETE")},
		)
		server := StartFunctionalServerWithConfig(
			t,
			dir,
			false,
			func(cfg *service.FactoryServiceConfig) {
				cfg.ProviderCommandRunnerOverride = runner
			},
		)

		server.WaitForCompleted(t, 10*time.Second)

		state := server.GetState(t)
		if state.Categories.Terminal != 1 {
			t.Fatalf("terminal token count = %d, want 1", state.Categories.Terminal)
		}
		if got := runner.CallCount(); got != 2 {
			t.Fatalf("provider command runner calls = %d, want 2", got)
		}
	})
}

func TestServiceConfigOverrideAlignment_ScriptCommandRunner(t *testing.T) {
	t.Run("ServiceHarness", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
		testutil.WriteSeedFile(t, dir, "task", []byte("script harness alignment"))

		runner := newRecordingCommandRunner("script alignment output")
		harness := testutil.NewServiceTestHarness(t, dir,
			testutil.WithFullWorkerPoolAndScriptWrap(),
			testutil.WithCommandRunner(runner),
		)

		harness.RunUntilComplete(t, 10*time.Second)

		harness.Assert().
			HasTokenInPlace("task:done").
			HasNoTokenInPlace("task:failed")
		if got := runner.CallCount(); got != 1 {
			t.Fatalf("script command runner calls = %d, want 1", got)
		}
	})

	t.Run("FunctionalHTTPServer", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
		testutil.WriteSeedFile(t, dir, "task", []byte("script server alignment"))

		runner := newRecordingCommandRunner("script alignment output")
		server := StartFunctionalServerWithConfig(
			t,
			dir,
			false,
			func(cfg *service.FactoryServiceConfig) {
				cfg.CommandRunnerOverride = runner
			},
		)

		server.WaitForCompleted(t, 10*time.Second)

		state := server.GetState(t)
		if state.Categories.Terminal != 1 {
			t.Fatalf("terminal token count = %d, want 1", state.Categories.Terminal)
		}
		if got := runner.CallCount(); got != 1 {
			t.Fatalf("script command runner calls = %d, want 1", got)
		}
	})
}

type recordingCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	requests []workers.CommandRequest
}

func newRecordingCommandRunner(stdout string) *recordingCommandRunner {
	return &recordingCommandRunner{stdout: []byte(stdout)}
}

func (r *recordingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))
	return workers.CommandResult{Stdout: append([]byte(nil), r.stdout...)}, nil
}

func (r *recordingCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *recordingCommandRunner) LastRequest() workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		panic("recordingCommandRunner: LastRequest() called with no requests")
	}
	return workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(r.requests[len(r.requests)-1]))
}

var _ workers.CommandRunner = (*recordingCommandRunner)(nil)
