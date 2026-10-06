package acceptance

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

type t7GatedProviderRunner struct {
	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	started  chan struct{}
	release  chan struct{}
	stopped  chan struct{}
}

func (r *t7GatedProviderRunner) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
	r.started, r.release, r.stopped = make(chan struct{}), make(chan struct{}), make(chan struct{})
}

func (r *t7GatedProviderRunner) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), r.requests...)
}

func (r *t7GatedProviderRunner) CallCount() int { return len(r.Requests()) }

func (r *t7GatedProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *t7GatedProviderRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, cloneS8CommandRequest(request))
	r.mu.Unlock()
	progress := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"t7-progress\",\"type\":\"command_execution\",\"command\":\"synthetic inspection\",\"aggregated_output\":\"T7 progress before detach\",\"exit_code\":0}}\n")
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, progress)
		if filepath.Base(request.WorkDir) == "t7-degraded" {
			// One committed progress record precedes this selected append loss.
			observer(platformprocess.OutputStreamStdout, []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"t7-lost-progress\",\"type\":\"command_execution\",\"command\":\"synthetic lost inspection\",\"exit_code\":0}}\n"))
		}
	}
	close(r.started)
	defer close(r.stopped)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-r.release:
		terminal := directCodexOutputWithoutSession("T7 detached attempt completed")
		if observer != nil {
			observer(platformprocess.OutputStreamStdout, terminal)
		}
		return platformprocess.CommandResult{Stdout: append(progress, terminal...)}, nil
	}
}

// F6-11 cancels a real host observer after command-edge progress. Admission
// and attempt ownership remain on the host; a separate CLI rejoins the ID.
func TestT7DetachedObserverLeavesHostedAttemptReadable(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "cli/you.worker-sessions.read")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-detach")
	defer scenario.close(t)
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	ctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
	defer stop()
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id := scenarioScopedID(scenario, "detached-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "detached controlled prompt",
	})
	path := filepath.Join(scenario.workingDirectory, "detach.json")
	writeInvokeContinueJSON(t, path, document)
	observerCtx, detach := context.WithCancel(ctx)
	defer detach()
	inputs := t7RemoteCLIInputs(scenario, observerCtx, fixture.baseURL, "invoke", "--execution", path)
	done := make(chan error, 1)
	go func() { done <- fixture.process.Execute(inputs.Input) }()
	t19AwaitSignal(t, ctx, runner.started, "hosted command progress")
	detach()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("detached synchronous observer succeeded")
		}
	case <-ctx.Done():
		t.Fatal("detached observer did not return")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_INVOKE_INTERRUPTED")
	show := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "show", "--worker-session-id", id)
	if err := fixture.process.Execute(show.Input); err != nil || !strings.Contains(show.Stdout(), `"state":"RUNNING"`) {
		t.Fatalf("detached live show: %v: %s %s", err, show.Stdout(), show.Stderr())
	}
	logs := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "logs", "--worker-session-id", id)
	if err := fixture.process.Execute(logs.Input); err != nil || !strings.Contains(logs.Stdout(), "T7 progress before detach") {
		t.Fatalf("detached prefix: %v: %s %s", err, logs.Stdout(), logs.Stderr())
	}
	close(runner.release)
	joined := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(joined.Input); err != nil {
		t.Fatalf("rejoin hosted attempt: %v: %s", err, joined.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, joined.Stdout(), &result)
	if result.WorkerSessionID != id || result.State != "COMPLETED" || result.Output != "T7 detached attempt completed" || runner.CallCount() != 1 {
		t.Fatalf("rejoined attempt = %#v, calls %d", result, runner.CallCount())
	}
	t7AssertDetachedTerminalCapture(t, ctx, fixture.baseURL, id)
}

func t7AssertDetachedTerminalCapture(t *testing.T, ctx context.Context, baseURL, id string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, "T7 progress before detach") || !strings.Contains(body, `"health":"COMPLETE"`) {
		t.Fatalf("detached terminal capture = %d %s", status, body)
	}
}

func t7ReleaseAndJoin(t *testing.T, ctx context.Context, runner *t7GatedProviderRunner) func() {
	t.Helper()
	// Release and join only this scenario's command on assertion failure.
	return func() {
		select {
		case <-runner.release:
		default:
			close(runner.release)
		}
		if runner.CallCount() > 0 {
			t19AwaitSignal(t, ctx, runner.stopped, "detached provider cleanup")
		}
	}
}

func t7RemoteCLIInputs(scenario *invokeContinueScenario, ctx context.Context, baseURL string, args ...string) *support.CapturedInputs {
	command := append([]string{"you", "--json", "--remote", "--server", baseURL, "worker-sessions"}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	return inputs
}

// A closed local listener supplies an actual refused connection without
// involving a real provider or relying on an arbitrary machine port.
func TestT7UnreachableHostNeverFallsBackToLocalProvider(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-unreachable")
	defer scenario.close(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	path := filepath.Join(scenario.workingDirectory, "unreachable.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		workingDirectory: scenario.workingDirectory, userMessage: "never execute locally",
	})
	inputs := t7RemoteCLIInputs(scenario, ctx, endpoint, "invoke", "--execution", path)
	if err := fixture.process.Execute(inputs.Input); err == nil {
		t.Fatal("unreachable host accepted invocation")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "FACTORY_UNREACHABLE")
	if scenario.providerRunner.CallCount() != 0 || inputs.Stdout() != "" || strings.Contains(inputs.Stderr(), "synthetic-password") || strings.Contains(inputs.Stderr(), "synthetic-user") {
		t.Fatalf("unreachable invocation leaked credentials or fell back: %s, calls %d", inputs.Stderr(), scenario.providerRunner.CallCount())
	}
}
