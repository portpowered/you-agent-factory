package wire

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestNewLoggingCommandRunnerForwardsExecutionAndPreservesNilFallbacks(t *testing.T) {
	t.Parallel()

	next := &wirePlatformCommandRunner{
		result: platformprocess.CommandResult{Stdout: []byte("logged output"), ExitCode: 3},
	}
	if got := NewLoggingCommandRunner(nil, logging.NoopLogger{}, time.Now); got != nil {
		t.Fatalf("NewLoggingCommandRunner(nil) = %T, want nil", got)
	}
	if got := NewLoggingCommandRunner(next, logging.NoopLogger{}, nil); got != next {
		t.Fatalf("NewLoggingCommandRunner(missing clock) = %T, want the injected runner", got)
	}

	runner := NewLoggingCommandRunner(next, logging.NoopLogger{}, func() time.Time {
		return time.Unix(100, 0)
	})
	result, err := runner.Run(context.Background(), platformprocess.CommandRequest{
		Command: "worker-command",
		Args:    []string{"--json"},
		WorkDir: "factory",
	})
	if err != nil {
		t.Fatalf("logged Run() error = %v", err)
	}
	if !reflect.DeepEqual(result, next.result) {
		t.Fatalf("logged Run() result = %#v, want %#v", result, next.result)
	}
	if next.calls != 1 || next.request.Command != "worker-command" || next.request.WorkDir != "factory" {
		t.Fatalf("logged runner request/calls = %#v/%d, want one forwarded request", next.request, next.calls)
	}
}

func TestNewProviderCommandRunnerProjectsRequestsAndBufferedOutput(t *testing.T) {
	t.Parallel()

	next := &wireWorkerCommandRunner{
		result: workerprocess.CommandResult{
			Stdout:   []byte("stdout"),
			Stderr:   []byte("stderr"),
			ExitCode: 7,
		},
	}
	runner := NewProviderCommandRunner(workerprocess.ProjectPlatformCommandRunner(next))
	request := providers.CommandRequest{
		Command:          "codex",
		Args:             []string{"exec", "--json"},
		Stdin:            []byte("prompt"),
		Env:              []string{"MODE=test"},
		WorkDir:          "factory",
		FactorySessionID: "factory-session-1",
		AttemptID:        "attempt-fallback",
		TransitionID:     "transition-1",
		WorkerType:       "agent",
		WorkstationName:  "reviewer",
		ProjectID:        "project-1",
		Execution:        work.ExecutionMetadata{RequestID: "request-1"},
		ExecutionLogger:  logging.NoopLogger{},
	}

	result, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("provider Run() error = %v", err)
	}
	if !reflect.DeepEqual(result.Stdout, next.result.Stdout) || !reflect.DeepEqual(result.Stderr, next.result.Stderr) || result.ExitCode != next.result.ExitCode {
		t.Fatalf("provider Run() result = %#v, want %#v", result, next.result)
	}
	assertProjectedWorkerRequest(t, next.request, request)

	var chunks []string
	result, err = runner.RunStreaming(context.Background(), request, func(stream string, chunk []byte) error {
		chunks = append(chunks, stream+":"+string(chunk))
		return nil
	})
	if err != nil {
		t.Fatalf("buffered provider RunStreaming() error = %v", err)
	}
	if !reflect.DeepEqual(result.Stdout, next.result.Stdout) || !reflect.DeepEqual(result.Stderr, next.result.Stderr) || result.ExitCode != next.result.ExitCode {
		t.Fatalf("buffered provider RunStreaming() result = %#v, want %#v", result, next.result)
	}
	if !reflect.DeepEqual(chunks, []string{
		platformprocess.OutputStreamStdout + ":stdout",
		platformprocess.OutputStreamStderr + ":stderr",
	}) {
		t.Fatalf("buffered provider output chunks = %#v", chunks)
	}
}

func TestNewProviderCommandRunnerForwardsStreamingOutputAndRejectsMissingRunner(t *testing.T) {
	t.Parallel()

	next := &wireStreamingWorkerCommandRunner{
		result: workerprocess.CommandResult{Stdout: []byte("live"), ExitCode: 0},
	}
	runner := NewProviderCommandRunner(workerprocess.ProjectPlatformCommandRunner(next))
	var chunks []string
	result, err := runner.RunStreaming(
		context.Background(),
		providers.CommandRequest{Command: "codex", DispatchID: "dispatch-live"},
		func(stream string, chunk []byte) error {
			chunks = append(chunks, stream+":"+string(chunk))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("streaming provider RunStreaming() error = %v", err)
	}
	if !reflect.DeepEqual(result.Stdout, next.result.Stdout) || !reflect.DeepEqual(result.Stderr, next.result.Stderr) || result.ExitCode != next.result.ExitCode || !reflect.DeepEqual(chunks, []string{"stdout:live"}) {
		t.Fatalf("streaming provider result/chunks = %#v/%#v", result, chunks)
	}
	if next.request.DispatchID != "dispatch-live" {
		t.Fatalf("streaming provider dispatch ID = %q, want dispatch-live", next.request.DispatchID)
	}

	missing := NewProviderCommandRunner(nil)
	if _, err := missing.Run(context.Background(), providers.CommandRequest{}); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("missing provider Run() error = %v, want required-runner error", err)
	}
	if _, err := missing.RunStreaming(context.Background(), providers.CommandRequest{}, nil); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("missing provider RunStreaming() error = %v, want required-runner error", err)
	}
}

func assertProjectedWorkerRequest(t *testing.T, got workerprocess.CommandRequest, want providers.CommandRequest) {
	t.Helper()
	if got.Command != want.Command || !reflect.DeepEqual(got.Args, want.Args) ||
		!reflect.DeepEqual(got.Stdin, want.Stdin) || !reflect.DeepEqual(got.Env, want.Env) ||
		got.WorkDir != want.WorkDir || got.FactorySessionID != want.FactorySessionID || got.DispatchID != want.AttemptID ||
		got.TransitionID != want.TransitionID || got.WorkerType != want.WorkerType ||
		got.WorkstationName != want.WorkstationName || got.ProjectID != want.ProjectID ||
		!reflect.DeepEqual(got.Execution, want.Execution) || got.ExecutionLogger == nil {
		t.Fatalf("projected worker request = %#v, want provider request projection %#v", got, want)
	}
}

type wirePlatformCommandRunner struct {
	request platformprocess.CommandRequest
	result  platformprocess.CommandResult
	calls   int
}

func (runner *wirePlatformCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.request = request
	runner.calls++
	return runner.result, nil
}

type wireWorkerCommandRunner struct {
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
}

func (runner *wireWorkerCommandRunner) Run(
	_ context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	runner.request = workerprocess.CloneCommandRequest(request)
	return runner.result, nil
}

type wireStreamingWorkerCommandRunner struct {
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
}

func (runner *wireStreamingWorkerCommandRunner) Run(
	_ context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	runner.request = workerprocess.CloneCommandRequest(request)
	return runner.result, nil
}

func (runner *wireStreamingWorkerCommandRunner) RunStreaming(
	_ context.Context,
	request workerprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	runner.request = workerprocess.CloneCommandRequest(request)
	if observer != nil && len(runner.result.Stdout) > 0 {
		observer(platformprocess.OutputStreamStdout, runner.result.Stdout)
	}
	return runner.result, nil
}

func TestNewProviderCommandRunnerRetainsObserverFailureAndNativeError(t *testing.T) {
	t.Parallel()
	observerFailure := errors.New("consumer stopped")
	nativeFailure := errors.New("command failed")
	for _, streaming := range []bool{false, true} {
		name := "buffered"
		if streaming {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var next platformprocess.CommandRunner = canonicalCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
				return platformprocess.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr"), ExitCode: 7}, nil
			})
			if streaming {
				next = streamingCanonicalCommandRunner{chunk: "stdout"}
			}
			runner := NewProviderCommandRunner(next)
			calls := 0
			result, err := runner.RunStreaming(t.Context(), providers.CommandRequest{}, func(string, []byte) error {
				calls++
				return observerFailure
			})
			if !errors.Is(err, observerFailure) || calls != 1 || len(result.Stdout) == 0 {
				t.Fatalf("consumer failure: result=%#v error=%v calls=%d", result, err, calls)
			}
		})
	}
	next := canonicalCommandRunnerFunc(func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		return platformprocess.CommandResult{Stdout: []byte("partial"), ExitCode: 9}, nativeFailure
	})
	result, err := NewProviderCommandRunner(next).RunStreaming(t.Context(), providers.CommandRequest{}, func(string, []byte) error {
		return observerFailure
	})
	if !errors.Is(err, nativeFailure) || result.ExitCode != 9 || string(result.Stdout) != "partial" {
		t.Fatalf("native failure: result=%#v error=%v", result, err)
	}
}

type bridgeOwnedControl struct{}

func (*bridgeOwnedControl) ForceKill(context.Context) (bool, error) { return false, nil }

func TestProviderCommandBridgePublishesExactOwnedCapability(t *testing.T) {
	t.Parallel()
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint("streaming=", streaming), func(t *testing.T) {
			t.Parallel()
			control := &bridgeOwnedControl{}
			var observed platformprocess.OwnedProcessControl
			next := canonicalCommandRunnerFunc(func(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
				if request.OwnedProcessObserver == nil {
					t.Fatal("Providers-to-platform bridge lost the owned capability observer")
				}
				request.OwnedProcessObserver(control)
				return platformprocess.CommandResult{}, nil
			})
			request := providers.CommandRequest{AttemptID: "physical-attempt", OwnedProcessObserver: func(got platformprocess.OwnedProcessControl) { observed = got }}
			runner := NewProviderCommandRunner(next)
			var err error
			if streaming {
				_, err = runner.RunStreaming(t.Context(), request, nil)
			} else {
				_, err = runner.Run(t.Context(), request)
			}
			if err != nil || observed != control {
				t.Fatalf("capability publication = %v, %v; want exact runner capability", observed, err)
			}
		})
	}
}
