package script

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func emptyDocs(string) (map[string]string, error) {
	return map[string]string{}, nil
}

func testDependencies(
	commandRunner workerprocess.CommandRunner,
	factoryDocs workers.FactoryDocsLoader,
) Dependencies {
	return Dependencies{
		CommandRunner: commandRunner,
		FactoryDocs:   factoryDocs,
		Now:           func() time.Time { return time.Unix(0, 0) },
		Publish:       func(workers.ProgressFragment) {},
		Record:        func(workers.ScriptEvent) {},
	}
}

type outputChunk struct {
	stream  string
	payload string
}

type streamingCommandEdge struct {
	mu           sync.Mutex
	observations *observationLog
	request      workerprocess.CommandRequest
	chunks       []outputChunk
	result       workerprocess.CommandResult
	err          error
}

func (edge *streamingCommandEdge) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	return edge.RunStreaming(ctx, request, nil)
}

func (edge *streamingCommandEdge) RunStreaming(
	_ context.Context,
	request workerprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	edge.mu.Lock()
	edge.request = workerprocess.CloneCommandRequest(request)
	edge.mu.Unlock()
	if edge.observations != nil {
		edge.observations.Append("command")
	}
	for _, chunk := range edge.chunks {
		if observer != nil {
			observer(chunk.stream, []byte(chunk.payload))
		}
	}
	return workerprocess.CommandResult{
		Stdout:   append([]byte(nil), edge.result.Stdout...),
		Stderr:   append([]byte(nil), edge.result.Stderr...),
		ExitCode: edge.result.ExitCode,
	}, edge.err
}

func (edge *streamingCommandEdge) Request() workerprocess.CommandRequest {
	edge.mu.Lock()
	defer edge.mu.Unlock()
	return workerprocess.CloneCommandRequest(edge.request)
}

type observationLog struct {
	mu       sync.Mutex
	values   []string
	terminal workers.ScriptResponseEventPayload
}

func (log *observationLog) Append(value string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.values = append(log.values, value)
}

func (log *observationLog) Values() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.values...)
}

func (log *observationLog) SetTerminal(terminal workers.ScriptResponseEventPayload) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.terminal = terminal
	if terminal.ExitCode != nil {
		exitCode := *terminal.ExitCode
		log.terminal.ExitCode = &exitCode
	}
}

func (log *observationLog) Terminal() workers.ScriptResponseEventPayload {
	log.mu.Lock()
	defer log.mu.Unlock()
	terminal := log.terminal
	if log.terminal.ExitCode != nil {
		exitCode := *log.terminal.ExitCode
		terminal.ExitCode = &exitCode
	}
	return terminal
}

type sequenceClock struct {
	mu    sync.Mutex
	times []time.Time
}

func (clock *sequenceClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if len(clock.times) == 0 {
		return time.Time{}
	}
	value := clock.times[0]
	clock.times = clock.times[1:]
	return value
}

type nonStreamingCommandRunner struct {
	result workerprocess.CommandResult
	err    error
}

func (runner nonStreamingCommandRunner) Run(
	context.Context,
	workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	return runner.result, runner.err
}

type captureCommandRunner struct {
	mu      sync.Mutex
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
	calls   int
}

func (runner *captureCommandRunner) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *captureCommandRunner) RunStreaming(
	_ context.Context,
	request workerprocess.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	runner.request = workerprocess.CloneCommandRequest(request)
	return runner.result, nil
}

func (runner *captureCommandRunner) Request() workerprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return workerprocess.CloneCommandRequest(runner.request)
}

func (runner *captureCommandRunner) Calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func assertFailureType(t *testing.T, err error, want workers.WorkFailureType) {
	t.Helper()
	var failure *workers.ProviderError
	if !errors.As(err, &failure) || failure.Type != want {
		t.Fatalf("error = %#v, want ProviderError type %q", err, want)
	}
}

func assertEnvAbsent(t *testing.T, env []string, name string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			t.Fatalf("environment %s = %#v, want absent when Factory declares bounded env", name, env)
		}
	}
}

func assertEnv(t *testing.T, env []string, name, want string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range env {
		if entry == prefix+want {
			return
		}
	}
	t.Fatalf("environment %s = %#v, want %q", name, env, want)
}

func assertEnvCount(t *testing.T, env []string, name string, want int) {
	t.Helper()
	prefix := name + "="
	count := 0
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			count++
		}
	}
	if count != want {
		t.Fatalf("environment %s count = %d, want %d in %#v", name, count, want, env)
	}
}
