package wire

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestWorkersWireLoggingBridgePreservesRequests(t *testing.T) {
	t.Parallel()

	var observed platformprocess.CommandRequest
	next := canonicalCommandRunnerFunc(func(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		observed = request
		return platformprocess.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}, nil
	})
	logged := NewLoggingCommandRunner(next, logging.NoopLogger{}, func() time.Time { return time.Unix(10, 0) })
	if logged == nil {
		t.Fatal("NewLoggingCommandRunner() = nil, want wrapped runner")
	}
	result, err := logged.Run(context.Background(), platformprocess.CommandRequest{
		Command: "worker", Args: []string{"--flag"}, WorkDir: "factory",
	})
	if err != nil || string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" {
		t.Fatalf("logging bridge Run() = %#v, %v", result, err)
	}
	if observed.Command != "worker" || !reflect.DeepEqual(observed.Args, []string{"--flag"}) || observed.WorkDir != "factory" {
		t.Fatalf("logging bridge request = %#v", observed)
	}
	if NewLoggingCommandRunner(nil, logging.NoopLogger{}, func() time.Time { return time.Unix(10, 0) }) != nil {
		t.Fatal("NewLoggingCommandRunner(nil) should return nil")
	}
	if NewLoggingCommandRunner(next, logging.NoopLogger{}, nil) == nil {
		t.Fatal("NewLoggingCommandRunner(nil clock) should preserve the next runner")
	}

}

func TestWorkersWireProviderRequestMappingPreservesFields(t *testing.T) {
	t.Parallel()

	request := providerCoverageCommandRequest()
	converted := workerCommandRequest(request)
	if converted.DispatchID != request.AttemptID || converted.Command != request.Command ||
		!reflect.DeepEqual(converted.Args, request.Args) || string(converted.Stdin) != string(request.Stdin) ||
		converted.WorkDir != request.WorkDir || converted.TransitionID != request.TransitionID ||
		converted.WorkerType != request.WorkerType || converted.WorkstationName != request.WorkstationName ||
		converted.ProjectID != request.ProjectID {
		t.Fatalf("workerCommandRequest() = %#v, want detached provider request mapping", converted)
	}
}

func TestWorkersWireProviderRunnerBuffersOutput(t *testing.T) {
	t.Parallel()

	next := canonicalCommandRunnerFunc(func(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		return platformprocess.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}, nil
	})
	providerRunner := NewProviderCommandRunner(next)
	request := providerCoverageCommandRequest()
	result, err := providerRunner.Run(context.Background(), request)
	if err != nil || string(result.Stdout) != "stdout" {
		t.Fatalf("provider command Run() = %#v, %v", result, err)
	}
	var chunks []recordedOutputChunk
	result, err = providerRunner.RunStreaming(context.Background(), request, func(stream string, chunk []byte) error {
		recordOutputChunks(&chunks)(stream, chunk)
		return nil
	})
	if err != nil || string(result.Stderr) != "stderr" {
		t.Fatalf("buffered provider RunStreaming() = %#v, %v", result, err)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "stdout"},
		{stream: platformprocess.OutputStreamStderr, chunk: "stderr"},
	})
}

func TestWorkersWireProviderRunnerStreamsOutput(t *testing.T) {
	t.Parallel()

	providerRunner := NewProviderCommandRunner(streamingCanonicalCommandRunner{chunk: "streamed"})
	var chunks []recordedOutputChunk
	result, err := providerRunner.RunStreaming(context.Background(), providerCoverageCommandRequest(), func(stream string, chunk []byte) error {
		recordOutputChunks(&chunks)(stream, chunk)
		return nil
	})
	if err != nil || string(result.Stdout) != "streamed provider-worker" {
		t.Fatalf("streaming provider RunStreaming() = %#v, %v", result, err)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{{stream: platformprocess.OutputStreamStdout, chunk: "streamed"}})
}

func TestWorkersWireProviderRunnerRequiresDelegate(t *testing.T) {
	t.Parallel()
	request := providerCoverageCommandRequest()
	if _, err := (providerCommandRunner{}).Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("nil provider Run() error = %v", err)
	}
	if _, err := (providerCommandRunner{}).RunStreaming(context.Background(), request, nil); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("nil provider RunStreaming() error = %v", err)
	}
}

func providerCoverageCommandRequest() providers.CommandRequest {
	return providers.CommandRequest{
		Command:                  "provider-worker",
		Args:                     []string{"--model", "tts"},
		Stdin:                    []byte("input"),
		Env:                      []string{"MODE=test"},
		WorkDir:                  "factory",
		AttemptID:                "attempt-1",
		TransitionID:             "execute",
		WorkerType:               "inference",
		WorkstationName:          "tts-executor",
		ProjectID:                "project",
		InputTokens:              []any{"token"},
		InputBindings:            map[string][]string{"text": {"input-1"}},
		Execution:                work.ExecutionMetadata{},
		ExecutionLogger:          logging.NoopLogger{},
		ProcessLifecycleObserver: nil,
	}
}
