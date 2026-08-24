package wire

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestCanonicalRuntimeBridgeHelpers(t *testing.T) {
	t.Parallel()

	hooks := LocalRuntimeHooks()
	if hooks.MarkLoadFinished == nil || hooks.MarkLoadRequested == nil {
		t.Fatal("LocalRuntimeHooks() returned incomplete model recording hooks")
	}
	if NewMockCommandRunner(nil, nil, stubCanonicalCommandRunner{}) == nil {
		t.Fatal("NewMockCommandRunner() returned nil")
	}

	resolved, err := ResolveTemplateFields(
		"{{.Context.WorkDir}}/sub",
		map[string]string{"NAME": "{{.Context.WorkDir}}"},
		nil,
		&workers.Context{WorkDirectory: "reviewer"},
		"",
	)
	if err != nil {
		t.Fatalf("ResolveTemplateFields() error = %v", err)
	}
	if resolved.WorkingDirectory != "reviewer/sub" || resolved.Env["NAME"] != "reviewer" {
		t.Fatalf("resolved fields = %#v", resolved)
	}
}

func TestCanonicalMockCommandRunnerUsesDetachedOverride(t *testing.T) {
	t.Parallel()

	runner := NewContextualMockWorkerCommandRunner(canonicalCommandRunnerFunc(
		func(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			return platformprocess.CommandResult{Stdout: []byte("forwarded " + request.Command)}, nil
		},
	))
	result, err := runner.Run(context.Background(), platformprocess.CommandRequest{Command: "codex"})
	if err != nil || string(result.Stdout) != "forwarded codex" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}

	configured := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeAccept}}}
	ctx := workerexecution.WithMockWorkerOutputPolicy(
		workerexecution.WithMockWorkersConfig(context.Background(), configured),
		workers.OutputPolicy{DecisionEnvelope: true},
	)
	result, err = runner.Run(ctx, platformprocess.CommandRequest{Command: "codex"})
	if err != nil || !strings.Contains(string(result.Stdout), "ACCEPTED") {
		t.Fatalf("Run(mock) = %#v, %v", result, err)
	}
}

// TestCanonicalMockCommandRunnerStreamsDetachedOverrideOutput proves a
// request-scoped mock override still reaches the streaming observer. Direct
// Workers execution reads live output from this seam, so a mocked dispatch that
// returned its result without republishing it would leave the caller with a
// silent stream.
func TestCanonicalMockCommandRunnerStreamsDetachedOverrideOutput(t *testing.T) {
	t.Parallel()

	runner := canonicalStreamingMockCommandRunner(t, canonicalCommandRunnerFunc(
		func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			return platformprocess.CommandResult{}, errors.New("next runner must not run when a detached override matches")
		},
	))
	exitCode := 7
	ctx := workerexecution.WithMockWorkersConfig(context.Background(), &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stdout:   "override stdout",
				Stderr:   "override stderr",
				ExitCode: &exitCode,
			},
		}},
	})

	var chunks []recordedOutputChunk
	result, err := runner.RunStreaming(ctx, platformprocess.CommandRequest{Command: "review"}, recordOutputChunks(&chunks))
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if string(result.Stdout) != "override stdout" || string(result.Stderr) != "override stderr" {
		t.Fatalf("RunStreaming() result = %#v", result)
	}
	if result.ExitCode != exitCode {
		t.Fatalf("RunStreaming() exit code = %d, want %d", result.ExitCode, exitCode)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "override stdout"},
		{stream: platformprocess.OutputStreamStderr, chunk: "override stderr"},
	})
}

// TestCanonicalMockCommandRunnerDelegatesStreamingWithoutOverride proves the
// decorator hands the observer to a streaming-capable edge instead of buffering
// the command. Republishing the completed result here would duplicate every
// chunk the edge already emitted.
func TestCanonicalMockCommandRunnerDelegatesStreamingWithoutOverride(t *testing.T) {
	t.Parallel()

	runner := canonicalStreamingMockCommandRunner(t, streamingCanonicalCommandRunner{chunk: "live chunk"})

	var chunks []recordedOutputChunk
	result, err := runner.RunStreaming(
		context.Background(),
		platformprocess.CommandRequest{Command: "codex"},
		recordOutputChunks(&chunks),
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if string(result.Stdout) != "streamed codex" {
		t.Fatalf("RunStreaming() result = %#v", result)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "live chunk"},
	})
}

// TestCanonicalMockCommandRunnerPublishesCompletedOutputForBufferedEdge proves a
// non-streaming edge still produces observable stdout and stderr, and that a
// caller that supplies no observer is tolerated rather than panicking.
func TestCanonicalMockCommandRunnerPublishesCompletedOutputForBufferedEdge(t *testing.T) {
	t.Parallel()

	runner := canonicalStreamingMockCommandRunner(t, canonicalCommandRunnerFunc(
		func(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			return platformprocess.CommandResult{
				Stdout: []byte("forwarded " + request.Command),
				Stderr: []byte("forwarded diagnostics"),
			}, nil
		},
	))

	var chunks []recordedOutputChunk
	result, err := runner.RunStreaming(
		context.Background(),
		platformprocess.CommandRequest{Command: "codex"},
		recordOutputChunks(&chunks),
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if string(result.Stdout) != "forwarded codex" {
		t.Fatalf("RunStreaming() result = %#v", result)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "forwarded codex"},
		{stream: platformprocess.OutputStreamStderr, chunk: "forwarded diagnostics"},
	})

	if _, err = runner.RunStreaming(context.Background(), platformprocess.CommandRequest{Command: "codex"}, nil); err != nil {
		t.Fatalf("RunStreaming(no observer) error = %v", err)
	}
}

// TestCanonicalMockCommandRunnerRequiresNextCommandRunner proves a missing
// command edge fails loudly instead of reporting an empty successful dispatch.
func TestCanonicalMockCommandRunnerRequiresNextCommandRunner(t *testing.T) {
	t.Parallel()

	runner := NewContextualMockWorkerCommandRunner(nil)
	_, err := runner.Run(context.Background(), platformprocess.CommandRequest{Command: "codex"})
	if err == nil || !strings.Contains(err.Error(), "next command runner is required") {
		t.Fatalf("Run(no next runner) error = %v, want a required-next-runner failure", err)
	}
}

func TestCanonicalProviderCommandRunnerProjectsRequestsAndStreaming(t *testing.T) {
	t.Parallel()

	next := &providerWireWorkerRunner{}
	runner := providerCommandRunner{runner: next}
	request := providerCommandRequest{
		Command:         "codex",
		Args:            []string{"--headless"},
		Stdin:           []byte("input"),
		Env:             []string{"MODE=test"},
		WorkDir:         "factory",
		DispatchID:      "dispatch-1",
		AttemptID:       "attempt-1",
		TransitionID:    "transition-1",
		WorkerType:      "agent",
		WorkstationName: "workstation-1",
		ProjectID:       "project-1",
		InputTokens:     []any{"token"},
		InputBindings:   map[string][]string{"input": {"token"}},
		Execution:       work.ExecutionMetadata{RequestID: "request-1"},
	}

	result, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("providerCommandRunner.Run() error = %v", err)
	}
	if string(result.Stdout) != "provider-run" {
		t.Fatalf("providerCommandRunner.Run() result = %#v", result)
	}
	if len(next.requests) != 1 || next.requests[0].DispatchID != "dispatch-1" {
		t.Fatalf("projected Run() requests = %#v", next.requests)
	}
	if next.requests[0].Command != request.Command || next.requests[0].WorkDir != request.WorkDir {
		t.Fatalf("projected Run() request = %#v", next.requests[0])
	}

	request.DispatchID = ""
	result, err = runner.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("providerCommandRunner.Run() fallback error = %v", err)
	}
	if len(next.requests) != 2 || next.requests[1].DispatchID != "attempt-1" {
		t.Fatalf("attempt fallback request = %#v", next.requests[1])
	}

	var chunks []recordedOutputChunk
	result, err = runner.RunStreaming(context.Background(), request, recordOutputChunks(&chunks))
	if err != nil {
		t.Fatalf("providerCommandRunner.RunStreaming() error = %v", err)
	}
	if string(result.Stdout) != "provider-stream" {
		t.Fatalf("providerCommandRunner.RunStreaming() result = %#v", result)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{{stream: platformprocess.OutputStreamStdout, chunk: "provider chunk"}})

	if _, ok := NewProviderCommandRunner(stubCanonicalCommandRunner{}).(providerCommandRunner); !ok {
		t.Fatal("NewProviderCommandRunner() returned an unexpected shape")
	}
}

func TestCanonicalProviderCommandRunnerHandlesMissingAndBufferedEdges(t *testing.T) {
	t.Parallel()

	var missing providerCommandRunner
	if _, err := missing.Run(context.Background(), providerCommandRequest{}); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("providerCommandRunner.Run(nil) error = %v", err)
	}
	if _, err := missing.RunStreaming(context.Background(), providerCommandRequest{}, nil); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("providerCommandRunner.RunStreaming(nil) error = %v", err)
	}

	buffered := providerCommandRunner{runner: providerWireBufferedWorkerRunner{}}
	var chunks []recordedOutputChunk
	result, err := buffered.RunStreaming(context.Background(), providerCommandRequest{}, recordOutputChunks(&chunks))
	if err != nil {
		t.Fatalf("buffered providerCommandRunner.RunStreaming() error = %v", err)
	}
	if string(result.Stdout) != "buffered stdout" || string(result.Stderr) != "buffered stderr" {
		t.Fatalf("buffered provider result = %#v", result)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "buffered stdout"},
		{stream: platformprocess.OutputStreamStderr, chunk: "buffered stderr"},
	})
}

func TestCanonicalLoggingAndProviderBridgesExposeRequiredEdges(t *testing.T) {
	t.Parallel()

	if got := NewLoggingCommandRunner(nil, nil, func() time.Time { return time.Unix(1, 0) }); got != nil {
		t.Fatalf("NewLoggingCommandRunner(nil runner) = %#v, want nil", got)
	}
	if got := NewLoggingCommandRunner(stubCanonicalCommandRunner{}, nil, nil); got == nil {
		t.Fatal("NewLoggingCommandRunner(missing clock) returned nil")
	}
	if got := NewLoggingCommandRunner(stubCanonicalCommandRunner{}, nil, func() time.Time { return time.Unix(1, 0) }); got == nil {
		t.Fatal("NewLoggingCommandRunner() returned nil")
	}

	service := &statelessTestProviders{}
	got, err := NewProviderFromCommandRunner(service, nil, nil, nil, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("NewProviderFromCommandRunner() error = %v", err)
	}
	if got != service {
		t.Fatalf("NewProviderFromCommandRunner() service = %#v, want original service", got)
	}
}

// TestCanonicalConductorInvocationIsTheRetainedDirectInvocationSuccessor
// characterizes the one direct-invocation constructor that still has a
// production caller (pkg/wire/session_runtime_providers.go). Its zero-caller
// siblings NewInvocation and NewInvocationWithProgress were retired, so this
// guard pins the surviving successor's observable construction contract: a
// usable executor when its required edges are supplied, and a loud failure
// naming the missing edge otherwise.
func TestCanonicalConductorInvocationIsTheRetainedDirectInvocationSuccessor(t *testing.T) {
	t.Parallel()

	executor, err := newCanonicalConductorInvocation(nil)
	if err != nil {
		t.Fatalf("NewConductorInvocationWithProgress() error = %v", err)
	}
	if executor == nil {
		t.Fatal("NewConductorInvocationWithProgress() returned no invocation executor")
	}

	published := make(chan workers.ProgressFragment, 1)
	withProgress, err := newCanonicalConductorInvocation(func(fragment workers.ProgressFragment) {
		select {
		case published <- fragment:
		default:
		}
	})
	if err != nil {
		t.Fatalf("NewConductorInvocationWithProgress(publisher) error = %v", err)
	}
	if withProgress == nil {
		t.Fatal("NewConductorInvocationWithProgress(publisher) returned no invocation executor")
	}
}

// TestCanonicalConductorInvocationRejectsMissingRequiredEdges proves the
// retained bridge refuses to build a half-wired executor. A constructor that
// returned a usable value here would defer the failure to dispatch time, where
// it would surface as a silent worker attempt instead of a construction error.
func TestCanonicalConductorInvocationRejectsMissingRequiredEdges(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		mutate  func(*canonicalInvocationEdges)
		wantErr string
	}{
		{
			name:    "command runner",
			mutate:  func(edges *canonicalInvocationEdges) { edges.commandRunner = nil },
			wantErr: "command runner is required",
		},
		{
			name:    "command clock",
			mutate:  func(edges *canonicalInvocationEdges) { edges.commandClock = nil },
			wantErr: "command clock is required",
		},
		{
			name:    "PTY allocator",
			mutate:  func(edges *canonicalInvocationEdges) { edges.allocator = nil },
			wantErr: "PTY allocator is required",
		},
		{
			name:    "Providers service",
			mutate:  func(edges *canonicalInvocationEdges) { edges.providers = nil },
			wantErr: "Providers service is required",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			edges := canonicalInvocationTestEdges()
			testCase.mutate(&edges)

			executor, err := NewConductorInvocationWithProgress(
				edges.providers,
				edges.commandRunner,
				edges.commandClock,
				edges.allocator,
				nil, nil, nil, nil, "",
				nil,
			)
			if err == nil {
				t.Fatalf("NewConductorInvocationWithProgress(no %s) error = nil, want a required-edge failure", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("NewConductorInvocationWithProgress(no %s) error = %v, want it to name %q", testCase.name, err, testCase.wantErr)
			}
			if executor != nil {
				t.Fatalf("NewConductorInvocationWithProgress(no %s) returned executor %#v, want nil", testCase.name, executor)
			}
		})
	}
}

// canonicalInvocationEdges collects the edges the retained direct-invocation
// constructor validates before it yields an executor.
type canonicalInvocationEdges struct {
	providers     providers.Service
	commandRunner platformprocess.CommandRunner
	commandClock  platformclock.Source
	allocator     workersinternal.PTYAllocator
}

func canonicalInvocationTestEdges() canonicalInvocationEdges {
	return canonicalInvocationEdges{
		providers:     &statelessTestProviders{},
		commandRunner: stubCanonicalCommandRunner{},
		commandClock:  platformclock.Real{},
		allocator:     &workersinternal.MockPTYAllocator{},
	}
}

func newCanonicalConductorInvocation(
	publisher workers.ProgressPublisher,
) (workers.InvocationExecutor, error) {
	edges := canonicalInvocationTestEdges()
	return NewConductorInvocationWithProgress(
		edges.providers,
		edges.commandRunner,
		edges.commandClock,
		edges.allocator,
		nil, nil, nil, nil, "",
		publisher,
	)
}

// canonicalStreamingCommandRunner is the contract Workers resolves from the
// composed command edge at dispatch time: buffered execution, live streaming,
// and diagnostics shaping.
type canonicalStreamingCommandRunner interface {
	platformprocess.CommandRunner
	RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
}

func canonicalStreamingMockCommandRunner(t *testing.T, next platformprocess.CommandRunner) canonicalStreamingCommandRunner {
	t.Helper()

	runner, ok := NewContextualMockWorkerCommandRunner(next).(canonicalStreamingCommandRunner)
	if !ok {
		t.Fatal("NewContextualMockWorkerCommandRunner() does not satisfy the canonical streaming command contract")
	}
	return runner
}

type recordedOutputChunk struct {
	stream string
	chunk  string
}

func recordOutputChunks(chunks *[]recordedOutputChunk) platformprocess.OutputChunkObserver {
	return func(stream string, chunk []byte) {
		*chunks = append(*chunks, recordedOutputChunk{stream: stream, chunk: string(chunk)})
	}
}

func assertStreamedChunks(t *testing.T, got, want []recordedOutputChunk) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamed chunks = %#v, want %#v", got, want)
	}
}

type streamingCanonicalCommandRunner struct {
	chunk string
}

func (streamingCanonicalCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("streaming edge must not fall back to buffered Run")
}

func (runner streamingCanonicalCommandRunner) RunStreaming(
	_ context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(runner.chunk))
	}
	return platformprocess.CommandResult{Stdout: []byte("streamed " + request.Command)}, nil
}

type stubCanonicalCommandRunner struct{}

func (stubCanonicalCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func (stubCanonicalCommandRunner) RunStreaming(
	context.Context,
	platformprocess.CommandRequest,
	platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

type canonicalCommandRunnerFunc func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)

func (runner canonicalCommandRunnerFunc) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner(ctx, request)
}

type providerWireWorkerRunner struct {
	requests []workerprocess.CommandRequest
}

func (runner *providerWireWorkerRunner) Run(_ context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	runner.requests = append(runner.requests, workerprocess.CloneCommandRequest(request))
	return workerprocess.CommandResult{Stdout: []byte("provider-run")}, nil
}

func (runner *providerWireWorkerRunner) RunStreaming(
	_ context.Context,
	request workerprocess.CommandRequest,
	observer workerprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	runner.requests = append(runner.requests, workerprocess.CloneCommandRequest(request))
	if observer != nil {
		observer(workerprocess.OutputStreamStdout, []byte("provider chunk"))
	}
	return workerprocess.CommandResult{Stdout: []byte("provider-stream")}, nil
}

type providerWireBufferedWorkerRunner struct{}

func (providerWireBufferedWorkerRunner) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	return workerprocess.CommandResult{
		Stdout: []byte("buffered stdout"),
		Stderr: []byte("buffered stderr"),
	}, nil
}
