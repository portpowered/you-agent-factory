package acceptance

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F7-C3 uses sequential production root processes over one scenario-owned
// store. No native provider files exist; only the command edge is substituted.
func TestCapturedProviderContinueAfterHostRestart(t *testing.T) {
	t.Parallel()
	dir, root := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "continued COMPLETE")},
	)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: "restart-source-request", workerSessionID: "restart-source", dispatchID: "restart-source-attempt",
		workingDirectory: dir, userMessage: "initial input",
	})
	invoke := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	invoke.Input.Env, invoke.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := first.process.Execute(invoke.Input); err != nil {
		t.Fatalf("source invoke: %v stdout=%s stderr=%s", err, invoke.Stdout(), invoke.Stderr())
	}
	var source directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &source)
	if !source.Accepted || source.State != "COMPLETED" || runner.CallCount() != 1 {
		t.Fatalf("source was not joined: %#v calls=%d", source, runner.CallCount())
	}
	awaitContinuationRestartLogs(t, first, home, dir, "restart-source")
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	continued := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "continue", "restart-source",
		"--request-id", "restart-continue-request", "--successor-worker-session-id", "restart-successor", "--user-message", "fresh host follow-up"})
	continued.Input.Env, continued.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := fresh.process.Execute(continued.Input); err != nil {
		t.Fatalf("fresh-host continuation: %v stdout=%s stderr=%s", err, continued.Stdout(), continued.Stderr())
	}
	assertContinuationRestartResult(t, continued.Stdout(), runner.Requests(), dir)
	logs := awaitContinuationRestartLogs(t, fresh, home, dir, "restart-successor")
	if !strings.Contains(logs, "restart-source") {
		t.Fatalf("successor logs omitted predecessor: %s", logs)
	}
	show := support.FakeInputs(t.Context(), []string{"you", "--server", fresh.baseURL, "--json", "worker-sessions", "show", "--worker-session-id", "restart-source"})
	show.Input.Env, show.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := fresh.process.Execute(show.Input); err != nil || !strings.Contains(show.Stdout(), "restart-successor") {
		t.Fatalf("archived source lost public successor link: %v stdout=%s stderr=%s", err, show.Stdout(), show.Stderr())
	}
}

func assertContinuationRestartResult(t *testing.T, stdout string, requests []platformprocess.CommandRequest, dir string) {
	t.Helper()
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, stdout, &result)
	if !result.Accepted || result.State != "COMPLETED" || result.SuccessorWorkerSessionID != "restart-successor" || !strings.Contains(result.Output, "continued COMPLETE") {
		t.Fatalf("fresh-host result = %#v", result)
	}
	if len(requests) != 2 || requests[1].WorkDir != dir || !strings.Contains(strings.Join(requests[1].Args, " "), "resume opaque-restart-thread") ||
		!strings.Contains(strings.Join(requests[1].Args, " ")+string(requests[1].Stdin), "fresh host follow-up") ||
		!strings.Contains(strings.Join(requests[1].Args, " "), "functional-model") {
		t.Fatalf("fresh-host native continuation = %#v", requests)
	}
}

func awaitContinuationRestartLogs(t *testing.T, host invokeContinueStartedProcess, home, dir, id string) string {
	t.Helper()
	// Following the capture joins its durable terminal before host shutdown,
	// rather than relying on a delay after the live session terminal result.
	logs := support.FakeInputs(t.Context(), []string{"you", "--server", host.baseURL, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow"})
	logs.Input.Env, logs.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(logs.Input); err != nil {
		t.Fatalf("joined captured logs: %v %s", err, logs.Stderr())
	}
	if !strings.Contains(logs.Stdout(), "opaque-restart-thread") {
		t.Fatalf("captured logs omitted native identity: %s", logs.Stdout())
	}
	return logs.Stdout()
}

func startContinuationRestartHost(t *testing.T, root, host, home string, route *invokeContinueStaticCommandRoute) invokeContinueStartedProcess {
	t.Helper()
	started, err := startInvokeContinuePackageProcess(t, root, host, home, route)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := started.command.stop(); err != nil {
			t.Error(err)
		}
		if err := started.process.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	return started
}
