package acceptance

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F7-R8/R10: reconstruct the production host over the same profile and replay
// both a successful admission and a stopped-source admission failure. Host
// reconstruction needs its own store; ordinary cells use the package process.
func TestInterruptExplicitModeReplayAfterHostRestart(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"provider", "recorded"} {
		for _, failed := range []bool{false, true} {
			name := mode + "/success"
			if failed {
				name = mode + "/admission-failure"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				runInterruptModeRestart(t, mode, failed)
			})
		}
	}
}

func runInterruptModeRestart(t *testing.T, mode string, failed bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	repositoryA, repositoryB := interruptRestartRepositories(t, root)
	runner := newS8InterruptProviderRunner(directCodexSessionOutput("session_fixture_codex_success", "Codex fixture answer COMPLETE"), repositoryA, repositoryB)
	nativeID := s8InterruptProviderSessionA
	if mode == "recorded" {
		nativeID = "fresh-recorded-native"
		runner.callFor(repositoryA.path, s8InterruptCallASuccessor).sessionID = nativeID
	}
	t.Cleanup(runner.releaseAll)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: repositoryA.path, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	env := invokeContinueEnvironment(home)
	invokeInterruptRestartSource(t, ctx, first, repositoryA.path, env, runner)
	successor := "successor"
	if failed {
		successor = "interrupt-admission-failure-successor"
	}
	args := []string{"you", "--json", "worker-sessions", "interrupt", "source", "--request-id", "interrupt-request",
		"--successor-worker-session-id", successor, "--replacement-message", s8ReplacementMessage, "--resume-mode", mode, "--async"}
	request := support.FakeInputs(ctx, args)
	request.Input.Env, request.Input.WorkingDirectory = env, repositoryA.path
	completeInterruptBeforeRestart(t, ctx, first, home, repositoryA.path, successor, nativeID, runner, request, failed)
	runner.waitCanceled(t, repositoryA.path, s8InterruptCallAInitial)
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	replay := support.FakeInputs(ctx, args)
	replay.Input.Env, replay.Input.WorkingDirectory = env, repositoryA.path
	replayErr := fresh.process.Execute(replay.Input)
	assertInterruptRestartReplay(t, failed, request, replay, replayErr, runner)
	args[len(args)-2] = "recorded"
	if mode == "recorded" {
		args[len(args)-2] = "provider"
	}
	conflict := support.FakeInputs(ctx, args)
	conflict.Input.Env, conflict.Input.WorkingDirectory = env, repositoryA.path
	if err := fresh.process.Execute(conflict.Input); err == nil || !strings.Contains(conflict.Stderr(), "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT") {
		t.Fatalf("changed mode after restart: %v stderr=%s", err, conflict.Stderr())
	}
}

func assertInterruptRestartReplay(t *testing.T, failed bool, request, replay *support.CapturedInputs, replayErr error, runner *s8InterruptProviderRunner) {
	t.Helper()
	if failed {
		if replayErr == nil || replay.Stderr() != request.Stderr() || runner.CallCount() != 1 {
			t.Fatalf("failed replay: %v stderr=%s calls=%d", replayErr, replay.Stderr(), runner.CallCount())
		}
	} else {
		var original, reconstructed s8InterruptResult
		if err := json.Unmarshal([]byte(request.Stdout()), &original); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(replay.Stdout()), &reconstructed); err != nil {
			t.Fatal(err)
		}
		if replayErr != nil || original != reconstructed || runner.CallCount() != 2 {
			t.Fatalf("successful replay: %v original=%+v replay=%+v calls=%d", replayErr, original, reconstructed, runner.CallCount())
		}
	}
}

func invokeInterruptRestartSource(t *testing.T, ctx context.Context, first invokeContinueStartedProcess, repository string, env []string, runner *s8InterruptProviderRunner) {
	t.Helper()
	path := filepath.Join(repository, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: "source-request", workerSessionID: "source", dispatchID: "source-attempt",
		workingDirectory: repository, userMessage: "initial input",
	})
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
	invoke.Input.Env, invoke.Input.WorkingDirectory = env, repository
	if err := first.process.Execute(invoke.Input); err != nil {
		t.Fatalf("source invoke: %v stderr=%s", err, invoke.Stderr())
	}
	runner.waitStarted(t, repository, s8InterruptCallAInitial)
}

func interruptRestartRepositories(t *testing.T, root string) (s8Repository, s8Repository) {
	t.Helper()
	repositoryA, err := newS8RepositoryAt(filepath.Join(root, "repository-a"), s8RepositoryAMarker)
	if err != nil {
		t.Fatal(err)
	}
	repositoryB, err := newS8RepositoryAt(filepath.Join(root, "repository-b"), s8RepositoryBMarker)
	if err != nil {
		t.Fatal(err)
	}
	return repositoryA, repositoryB
}

func completeInterruptBeforeRestart(t *testing.T, ctx context.Context, first invokeContinueStartedProcess, home, repository, successor, nativeID string, runner *s8InterruptProviderRunner, request *support.CapturedInputs, failed bool) {
	t.Helper()
	err := first.process.Execute(request.Input)
	if failed {
		if err == nil || !strings.Contains(request.Stderr(), "SUCCESSOR_ADMISSION") {
			t.Fatalf("admission failure: %v stderr=%s", err, request.Stderr())
		}
	} else {
		if err != nil {
			t.Fatalf("interrupt: %v stderr=%s", err, request.Stderr())
		}
		runner.waitStarted(t, repository, s8InterruptCallASuccessor)
		runner.releaseAll()
		awaitContinuationRestartLogs(t, first, home, repository, successor, nativeID)
	}
}
