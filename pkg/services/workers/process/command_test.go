package process

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type recordingEffect struct {
	request platformprocess.CommandRequest
	result  platformprocess.CommandResult
}

func (r *recordingEffect) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.request = request
	if r.result.Stdout == nil {
		r.result.Stdout = []byte("ok")
	}
	return r.result, nil
}

type streamingRecordingEffect struct {
	recordingEffect
	chunks []string
}

func (r *streamingRecordingEffect) RunStreaming(
	_ context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	r.request = request
	observer(platformprocess.OutputStreamStderr, []byte("warn"))
	observer(platformprocess.OutputStreamStdout, []byte("ok"))
	r.chunks = append(r.chunks, "streamed")
	return platformprocess.CommandResult{Stdout: []byte("ok"), Stderr: []byte("warn")}, nil
}

type workerCommandRunnerFunc func(context.Context, CommandRequest) (CommandResult, error)

func (run workerCommandRunnerFunc) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return run(ctx, request)
}

type streamingWorkerRunner struct {
	called bool
}

func (r *streamingWorkerRunner) Run(
	ctx context.Context,
	request CommandRequest,
) (CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *streamingWorkerRunner) RunStreaming(
	_ context.Context,
	_ CommandRequest,
	observer OutputChunkObserver,
) (CommandResult, error) {
	r.called = true
	if observer != nil {
		observer(OutputStreamStdout, []byte("live"))
	}
	return CommandResult{Stdout: []byte("live")}, nil
}

func TestAdaptCommandRunnerProjectsOnlySubprocessEffectFields(t *testing.T) {
	effect := &recordingEffect{}
	runner := AdaptCommandRunner(effect)
	request := commandTestRequest()
	result, err := runner.Run(t.Context(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(result.Stdout) != "ok" {
		t.Fatalf("stdout = %q, want ok", result.Stdout)
	}
	want := platformprocess.CommandRequest{
		Command: request.Command, Args: request.Args, Stdin: request.Stdin,
		Env: request.Env, WorkDir: request.WorkDir,
	}
	if effect.request.Command != want.Command || effect.request.WorkDir != want.WorkDir ||
		string(effect.request.Stdin) != string(want.Stdin) || len(effect.request.Args) != len(want.Args) ||
		len(effect.request.Env) != len(want.Env) {
		t.Fatalf("effect request = %#v, want %#v", effect.request, want)
	}
}

func TestAdaptCommandRunnerPreservesOnlyImplementedStreamingCapability(t *testing.T) {
	streamingMethod := func(runner CommandRunner) bool {
		_, ok := runner.(interface {
			RunStreaming(context.Context, CommandRequest, OutputChunkObserver) (CommandResult, error)
		})
		return ok
	}
	if streamingMethod(AdaptCommandRunner(&recordingEffect{})) {
		t.Fatal("non-streaming platform effect unexpectedly exposes workers streaming")
	}
	if !streamingMethod(AdaptCommandRunner(&streamingRecordingEffect{})) {
		t.Fatal("streaming platform effect does not expose workers streaming")
	}
}

func TestLoggingCommandRunnerOwnsWorkCorrelationProjection(t *testing.T) {
	logger := &recordingLogger{}
	clock := &sequenceCommandClock{times: []time.Time{
		time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 20, 12, 0, 2, 0, time.UTC),
	}}
	runner := CommandRunnerWithLogging(AdaptCommandRunner(&recordingEffect{}), logger, clock)
	request := commandTestRequest()
	if _, err := runner.Run(t.Context(), request); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	completed := logger.byEvent("command_runner.completed")
	if len(completed) != 1 {
		t.Fatalf("completion logs = %d, want 1", len(completed))
	}
	fields := completed[0]
	if fields["duration_ms"] != int64(2000) {
		t.Fatalf("duration_ms = %#v, want 2000", fields["duration_ms"])
	}
	for key, want := range map[string]any{
		"request_id": request.Execution.RequestID,
		"trace_id":   request.Execution.TraceID,
		"work_id":    request.Execution.WorkIDs[0],
	} {
		if fields[key] != want {
			t.Fatalf("%s = %#v, want %#v", key, fields[key], want)
		}
	}
}

func TestLoggingCommandRunnerRequiresInjectedClock(t *testing.T) {
	runner := CommandRunnerWithLogging(AdaptCommandRunner(&recordingEffect{}), nil, nil)
	if _, err := runner.Run(t.Context(), commandTestRequest()); err == nil || err.Error() != "workers logging command clock is required" {
		t.Fatalf("Run() error = %v, want missing command clock", err)
	}
}

func TestStreamingAdaptedCommandRunnerForwardsStreamingEffect(t *testing.T) {
	effect := &streamingRecordingEffect{}
	var chunks []string
	result, err := (StreamingAdaptedCommandRunner{
		ExecCommandRunner: ExecCommandRunner{Runner: effect},
	}).RunStreaming(
		t.Context(),
		commandTestRequest(),
		func(stream string, chunk []byte) {
			chunks = append(chunks, string(stream)+":"+string(chunk))
		},
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if len(effect.chunks) != 1 || string(result.Stdout) != "ok" ||
		len(chunks) != 2 || chunks[0] != "stderr:warn" || chunks[1] != "stdout:ok" {
		t.Fatalf("streaming result = %#v chunks = %#v effect = %#v", result, chunks, effect.chunks)
	}

	if _, err := (StreamingAdaptedCommandRunner{}).RunStreaming(
		t.Context(), commandTestRequest(), nil,
	); err == nil {
		t.Fatal("RunStreaming() error = nil, want missing process runner")
	}
}

func TestLoggingCommandRunnerRunStreamingSupportsStreamingAndFallbackEdges(t *testing.T) {
	newClock := func() *sequenceCommandClock {
		return &sequenceCommandClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}}
	}

	t.Run("streaming", func(t *testing.T) {
		edge := &streamingWorkerRunner{}
		var chunks []string
		result, err := (LoggingCommandRunner{Runner: edge, Clock: newClock()}).RunStreaming(
			t.Context(),
			commandTestRequest(),
			func(_ string, chunk []byte) {
				chunks = append(chunks, string(chunk))
			},
		)
		if err != nil {
			t.Fatalf("RunStreaming() error = %v", err)
		}
		if !edge.called || string(result.Stdout) != "live" || len(chunks) != 1 || chunks[0] != "live" {
			t.Fatalf("streaming result = %#v chunks = %#v called = %t", result, chunks, edge.called)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		edge := workerCommandRunnerFunc(func(context.Context, CommandRequest) (CommandResult, error) {
			return CommandResult{Stdout: []byte("out"), Stderr: []byte("err")}, nil
		})
		var chunks []string
		_, err := (LoggingCommandRunner{Runner: edge, Clock: newClock()}).RunStreaming(
			t.Context(),
			commandTestRequest(),
			func(stream string, chunk []byte) {
				chunks = append(chunks, string(stream)+":"+string(chunk))
			},
		)
		if err != nil {
			t.Fatalf("RunStreaming() error = %v", err)
		}
		if len(chunks) != 2 || chunks[0] != "stdout:out" || chunks[1] != "stderr:err" {
			t.Fatalf("fallback chunks = %#v", chunks)
		}
	})

	if _, err := (LoggingCommandRunner{Clock: newClock()}).RunStreaming(
		t.Context(), commandTestRequest(), nil,
	); err == nil {
		t.Fatal("RunStreaming() error = nil, want missing command runner")
	}
	edge := workerCommandRunnerFunc(func(context.Context, CommandRequest) (CommandResult, error) {
		return CommandResult{}, nil
	})
	if _, err := (LoggingCommandRunner{Runner: edge}).RunStreaming(
		t.Context(), commandTestRequest(), nil,
	); err == nil {
		t.Fatal("RunStreaming() error = nil, want missing command clock")
	}
}

func TestStreamingExecCommandRunnerRequiresAndForwardsStreamingEdge(t *testing.T) {
	if _, err := (StreamingExecCommandRunner{}).Run(t.Context(), commandTestRequest()); err == nil {
		t.Fatal("Run() error = nil, want missing runner")
	}
	nonStreaming := workerCommandRunnerFunc(func(context.Context, CommandRequest) (CommandResult, error) {
		return CommandResult{}, nil
	})
	if _, err := (StreamingExecCommandRunner{Runner: nonStreaming}).Run(
		t.Context(), commandTestRequest(),
	); err == nil {
		t.Fatal("Run() error = nil, want unsupported streaming edge")
	}
	streaming := &streamingWorkerRunner{}
	result, err := (StreamingExecCommandRunner{Runner: streaming}).Run(t.Context(), commandTestRequest())
	if err != nil || string(result.Stdout) != "live" || !streaming.called {
		t.Fatalf("Run() result = %#v error = %v called = %t", result, err, streaming.called)
	}
}

func TestCommandRunnerWithLoggingPreservesExistingRunnerIdentity(t *testing.T) {
	clock := &sequenceCommandClock{times: []time.Time{time.Unix(1, 0)}}
	existing := &LoggingCommandRunner{Runner: AdaptCommandRunner(&recordingEffect{})}
	got := CommandRunnerWithLogging(existing, nil, clock)
	if got != existing {
		t.Fatalf("CommandRunnerWithLogging() = %p, want existing runner %p", got, existing)
	}
	if existing.Clock != clock {
		t.Fatalf("existing clock = %T, want injected clock", existing.Clock)
	}
}

type sequenceCommandClock struct {
	times []time.Time
	next  int
}

func (clock *sequenceCommandClock) Now() time.Time {
	if clock.next >= len(clock.times) {
		return clock.times[len(clock.times)-1]
	}
	value := clock.times[clock.next]
	clock.next++
	return value
}

func TestExecCommandRunnerAddsWorkContextToPlatformCleanupLogs(t *testing.T) {
	if os.Getenv("GO_WANT_WORKER_COMMAND_HELPER") == "1" {
		return
	}
	logger := &recordingLogger{}
	request := commandTestRequest()
	request.Command = os.Args[0]
	request.Args = []string{"-test.run=TestExecCommandRunnerAddsWorkContextToPlatformCleanupLogs"}
	request.Env = append(os.Environ(), "GO_WANT_WORKER_COMMAND_HELPER=1")
	request.WorkDir = t.TempDir()
	effect, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	if _, err := (ExecCommandRunner{Runner: effect, Logger: logger}).Run(t.Context(), request); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	completed := logger.byEvent("command_runner.cleanup_completed")
	if len(completed) == 0 {
		t.Fatal("missing cleanup completion log")
	}
	fields := completed[len(completed)-1]
	for key, want := range map[string]any{
		"dispatch_id":      request.DispatchID,
		"worker_type":      request.WorkerType,
		"workstation_name": request.WorkstationName,
		"request_id":       request.Execution.RequestID,
		"trace_id":         request.Execution.TraceID,
		"work_id":          request.Execution.WorkIDs[0],
	} {
		if fields[key] != want {
			t.Fatalf("%s = %#v, want %#v", key, fields[key], want)
		}
	}
}

func commandTestRequest() CommandRequest {
	return CommandRequest{
		Command: "worker-tool", Args: []string{"--fixture"}, Stdin: []byte("input"),
		Env: []string{"VISIBLE=1"}, WorkDir: "work-dir", DispatchID: "dispatch-1",
		WorkerType: "script", WorkstationName: "station-1",
		Execution: work.ExecutionMetadata{RequestID: "request-1", TraceID: "trace-1", WorkIDs: []string{"work-1"}},
	}
}

func TestProjectPlatformCommandRunnerRoundTripsAdaptedRunner(t *testing.T) {
	effect := &recordingEffect{}
	adapted := AdaptCommandRunner(effect)
	projected := ProjectPlatformCommandRunner(adapted)
	request := platformprocess.CommandRequest{
		Command: "codex",
		Args:    []string{"exec", "--json"},
		Stdin:   []byte("prompt"),
		WorkDir: t.TempDir(),
	}
	result, err := projected.Run(t.Context(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(result.Stdout) != "ok" {
		t.Fatalf("stdout = %q, want ok", result.Stdout)
	}
	if effect.request.Command != request.Command {
		t.Fatalf("effect command = %q, want %q", effect.request.Command, request.Command)
	}
}

func TestProjectPlatformCommandRunnerPreservesWorkersStreaming(t *testing.T) {
	streaming := &streamingWorkerRunner{}
	projected := ProjectPlatformCommandRunner(streaming)
	streamingPlatform, ok := projected.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if !ok {
		t.Fatal("projected runner does not expose streaming")
	}
	result, err := streamingPlatform.RunStreaming(
		t.Context(),
		platformprocess.CommandRequest{Command: "claude"},
		func(stream string, chunk []byte) {
			if stream != OutputStreamStdout || string(chunk) != "live" {
				t.Fatalf("chunk = (%q, %q), want stdout/live", stream, chunk)
			}
		},
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if !streaming.called {
		t.Fatal("workers streaming runner was not invoked")
	}
	if string(result.Stdout) != "live" {
		t.Fatalf("stdout = %q, want live", result.Stdout)
	}
}

type recordingLogger struct {
	entries []map[string]any
}

func (*recordingLogger) Debug(string, ...any)              {}
func (l *recordingLogger) Info(_ string, fields ...any)    { l.record(fields) }
func (l *recordingLogger) Warn(_ string, fields ...any)    { l.record(fields) }
func (*recordingLogger) Error(string, ...any)              {}
func (l *recordingLogger) Verbose(_ string, fields ...any) { l.record(fields) }
func (l *recordingLogger) record(fields []any) {
	record := make(map[string]any)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if ok {
			record[key] = fields[i+1]
		}
	}
	l.entries = append(l.entries, record)
}
func (l *recordingLogger) byEvent(event string) []map[string]any {
	var result []map[string]any
	for _, entry := range l.entries {
		if entry["event_name"] == event {
			result = append(result, entry)
		}
	}
	return result
}
