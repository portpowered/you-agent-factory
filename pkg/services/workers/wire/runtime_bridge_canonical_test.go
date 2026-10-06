package wire

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

func TestCanonicalRuntimeBridgeHelpers(t *testing.T) {
	t.Parallel()

	hooks := LocalRuntimeHooks()
	if hooks.MarkLoadFinished == nil || hooks.MarkLoadRequested == nil {
		t.Fatal("LocalRuntimeHooks() returned incomplete model recording hooks")
	}
	if NewMockCommandRunner(nil, nil, stubCanonicalCommandRunner{}, nil) == nil {
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
	), nil)
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

	runner := NewContextualMockWorkerCommandRunner(nil, nil)
	_, err := runner.Run(context.Background(), platformprocess.CommandRequest{Command: "codex"})
	if err == nil || !strings.Contains(err.Error(), "next command runner is required") {
		t.Fatalf("Run(no next runner) error = %v, want a required-next-runner failure", err)
	}
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

	runner, ok := NewContextualMockWorkerCommandRunner(next, nil).(canonicalStreamingCommandRunner)
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
