package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
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
	if observer != nil {
		observer(platformprocess.OutputStreamStderr, []byte("warn"))
		observer(platformprocess.OutputStreamStdout, []byte("ok"))
	}
	r.chunks = append(r.chunks, "streamed")
	return platformprocess.CommandResult{Stdout: []byte("ok"), Stderr: []byte("warn")}, nil
}

type workerCommandRunnerFunc func(context.Context, CommandRequest) (CommandResult, error)

func (run workerCommandRunnerFunc) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return run(ctx, request)
}

type streamingCommandRunnerFunc struct{ workerCommandRunnerFunc }

func (runner streamingCommandRunnerFunc) RunStreaming(ctx context.Context, request CommandRequest, observer OutputChunkObserver) (CommandResult, error) {
	result, err := runner.Run(ctx, request)
	publishCompleteOutput(observer, result)
	return result, err
}

type streamingWorkerRunner struct {
	called bool
}

type loggingCommandOutcomeCase struct {
	name       string
	result     CommandResult
	err        error
	wantStatus string
	wantLevel  string
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
		ExecutionScopeID: request.FactorySessionID,
	}
	if effect.request.Command != want.Command || effect.request.WorkDir != want.WorkDir ||
		effect.request.ExecutionScopeID != want.ExecutionScopeID ||
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

func TestLoggingCommandRunnerSeparatesFailureAndCancellationLevels(t *testing.T) {
	cases := []loggingCommandOutcomeCase{
		{name: "success", result: CommandResult{Stdout: []byte("ok")}, wantStatus: "succeeded", wantLevel: "info"},
		{name: "wrapped error", err: fmt.Errorf("collaborator: %w", errors.ErrUnsupported), wantStatus: "error", wantLevel: "error"},
		{name: "ordinary cancellation", err: context.Canceled, wantStatus: "canceled", wantLevel: "info"},
		{name: "process gone", result: CommandResult{CancellationReason: platformprocess.CancellationReasonProcessGone},
			err: context.Canceled, wantStatus: "error", wantLevel: "error"},
		{
			name:       "non-zero exit",
			result:     CommandResult{ExitCode: 7},
			wantStatus: "failed",
			wantLevel:  "error",
		},
		{
			name:       "timeout",
			err:        context.DeadlineExceeded,
			wantStatus: "timed_out",
			wantLevel:  "error",
		},
		{
			name:       "superseded cancellation",
			result:     CommandResult{CancellationReason: platformprocess.CancellationReasonSuperseded},
			err:        context.Canceled,
			wantStatus: "canceled",
			wantLevel:  "info",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
					t.Parallel()
					assertLoggingCommandOutcome(t, tc, streaming)
				})
			}
		})
	}
}

func assertLoggingCommandOutcome(t *testing.T, tc loggingCommandOutcomeCase, streaming bool) {
	t.Helper()
	logger := &recordingLogger{}
	request := commandTestRequest()
	request.Inputs = []workers.WorkInput{{WorkID: "work-1", Name: "checkout"}}
	runner := LoggingCommandRunner{
		Runner: workerCommandRunnerFunc(func(context.Context, CommandRequest) (CommandResult, error) {
			return tc.result, tc.err
		}),
		Logger: logger,
		Clock:  &sequenceCommandClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}},
	}

	var result CommandResult
	var err error
	if streaming {
		result, err = runner.RunStreaming(t.Context(), request, nil)
	} else {
		result, err = runner.Run(t.Context(), request)
	}
	assertCommandErrorIdentity(t, err, tc.err)
	if !reflect.DeepEqual(result, tc.result) {
		t.Fatalf("Run() = %#v, %v; want %#v, %v", result, err, tc.result, tc.err)
	}
	assertLoggingCommandCompletion(t, logger, tc)
}

func assertCommandErrorIdentity(t *testing.T, got, want error) {
	t.Helper()
	// Compare the original values as well as matching through the error chain:
	// wrapping preserves errors.Is but changes the returned error's identity.
	if reflect.ValueOf(got) != reflect.ValueOf(want) || !errors.Is(got, want) {
		t.Fatalf("error identity changed: got %v, want %v", got, want)
	}
}

func assertLoggingCommandCompletion(t *testing.T, logger *recordingLogger, tc loggingCommandOutcomeCase) {
	t.Helper()
	completion := logger.byEventWithLevel("command_runner.completed")
	if len(completion) != 1 {
		t.Fatalf("completion records = %#v, want one", completion)
	}
	if completion[0].level != tc.wantLevel {
		t.Fatalf("completion level = %q, want %q", completion[0].level, tc.wantLevel)
	}
	fields := completion[0].fields
	if fields["status"] != tc.wantStatus || fields["outcome"] != tc.wantStatus {
		t.Fatalf("completion status/outcome = %#v/%#v, want %q", fields["status"], fields["outcome"], tc.wantStatus)
	}
	if fields["work_name"] != "checkout" || fields["dispatch_id"] != "dispatch-1" || fields["transition_id"] != "transition-1" {
		t.Fatalf("completion correlation fields = %#v", fields)
	}
	if tc.wantLevel == "error" {
		if fields["duration_ms"] != int64(1000) {
			t.Fatal("failure duration changed")
		}
		assertLoggingFailureFields(t, fields, tc.err)
		return
	}
	if fields["duration_ms"] != int64(1000) {
		t.Fatalf("duration = %#v, want 1000", fields)
	}
	if tc.result.CancellationReason != "" && fields["cancellation_reason"] != string(tc.result.CancellationReason) {
		t.Fatalf("cancellation fields = %#v", fields)
	}
	if _, ok := fields["has_error"]; ok {
		t.Fatalf("non-failure log has error marker: %#v", fields)
	}
}

func assertLoggingFailureFields(t *testing.T, fields map[string]any, returnedError error) {
	t.Helper()
	if fields["failure_reason"] == nil {
		t.Fatalf("failure fields = %#v, want safe reason", fields)
	}
	if returnedError != nil && fields["has_error"] != true {
		t.Fatalf("failure fields = %#v, want error marker for returned error", fields)
	}
	if _, ok := fields["error"]; ok {
		t.Fatalf("failure log exposed raw error: %#v", fields["error"])
	}
}

func TestLoggingCommandRunnerRequiresInjectedClock(t *testing.T) {
	runner := CommandRunnerWithLogging(AdaptCommandRunner(&recordingEffect{}), logging.NoopLogger{}, nil)
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
		result, err := (LoggingCommandRunner{Runner: edge, Logger: logging.NoopLogger{}, Clock: newClock()}).RunStreaming(
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
		result, err := (LoggingCommandRunner{Runner: edge, Logger: logging.NoopLogger{}, Clock: newClock()}).RunStreaming(
			t.Context(),
			commandTestRequest(),
			func(stream string, chunk []byte) {
				chunks = append(chunks, string(stream)+":"+string(chunk))
				chunk[0] = 'X'
			},
		)
		if err != nil {
			t.Fatalf("RunStreaming() error = %v", err)
		}
		if len(chunks) != 2 || chunks[0] != "stdout:out" || chunks[1] != "stderr:err" {
			t.Fatalf("fallback chunks = %#v", chunks)
		}
		if string(result.Stdout) != "out" || string(result.Stderr) != "err" {
			t.Fatalf("observer changed returned buffers: %#v", result)
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

func TestCommandRunnerWithLoggingPreservesExistingRunnerIdentity(t *testing.T) {
	clock := &sequenceCommandClock{times: []time.Time{time.Unix(1, 0)}}
	existing := &LoggingCommandRunner{Runner: AdaptCommandRunner(&recordingEffect{})}
	got := CommandRunnerWithLogging(existing, logging.NoopLogger{}, clock)
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
	effect, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, nil)
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
		Env: []string{"VISIBLE=1"}, WorkDir: "work-dir", FactorySessionID: "factory-session-1",
		DispatchID: "dispatch-1", TransitionID: "transition-1",
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

func TestAdaptPlatformCommandRunnerPreservesPrivateRunnerIdentity(t *testing.T) {
	private := &privateRequestRecorder{}
	projected := ProjectPlatformCommandRunner(private)
	recovered := AdaptPlatformCommandRunner(projected)
	request := commandTestRequest()

	if _, err := recovered.Run(t.Context(), request); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if private.request.WorkerType != request.WorkerType ||
		private.request.WorkstationName != request.WorkstationName ||
		private.request.DispatchID != request.DispatchID {
		t.Fatalf("private request = %#v, want Workers correlation from %#v", private.request, request)
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

type privateRequestRecorder struct {
	request CommandRequest
}

func (runner *privateRequestRecorder) Run(
	_ context.Context,
	request CommandRequest,
) (CommandResult, error) {
	runner.request = request
	return CommandResult{}, nil
}

type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedWorkerLog
}

func (l *recordingLogger) Debug(_ string, fields ...any)   { l.record("debug", fields) }
func (l *recordingLogger) Info(_ string, fields ...any)    { l.record("info", fields) }
func (l *recordingLogger) Warn(_ string, fields ...any)    { l.record("warn", fields) }
func (l *recordingLogger) Error(_ string, fields ...any)   { l.record("error", fields) }
func (l *recordingLogger) Verbose(_ string, fields ...any) { l.record("verbose", fields) }

type recordedWorkerLog struct {
	level  string
	fields map[string]any
}

func (l *recordingLogger) record(level string, fields []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	record := make(map[string]any)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if ok {
			value := fields[i+1]
			if ids, ok := value.([]string); ok {
				value = append([]string(nil), ids...)
			}
			record[key] = value
		}
	}
	l.entries = append(l.entries, recordedWorkerLog{level: level, fields: record})
}
func (l *recordingLogger) byEvent(event string) []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var result []map[string]any
	for _, entry := range l.entries {
		if entry.fields["event_name"] == event {
			result = append(result, entry.fields)
		}
	}
	return result
}

func (l *recordingLogger) byEventWithLevel(event string) []recordedWorkerLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	var result []recordedWorkerLog
	for _, entry := range l.entries {
		if entry.fields["event_name"] == event {
			result = append(result, entry)
		}
	}
	return result
}

func TestLoggingCommandRunnerSelectedLogger(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, selection := range []string{"default", "override", "noop"} {
			t.Run(fmt.Sprintf("streaming=%t/%s", streaming, selection), func(t *testing.T) {
				t.Parallel()
				owner, override := &recordingLogger{}, &recordingLogger{}
				request := commandTestRequest()
				selected := logging.Logger(owner)
				if selection == "override" {
					request.ExecutionLogger = override
					selected = override
				}
				if selection == "noop" {
					request.ExecutionLogger = logging.NoopLogger{}
					selected = request.ExecutionLogger
				}
				want := CommandResult{Stdout: []byte("out"), Stderr: []byte("err"), ExitCode: 0}
				calls := 0
				var edge CommandRunner = workerCommandRunnerFunc(func(_ context.Context, got CommandRequest) (CommandResult, error) {
					calls++
					if got.ExecutionLogger != selected {
						t.Errorf("forwarded logger = %T, want selected effect", got.ExecutionLogger)
					}
					commandContextLogger(logging.NoopLogger{}, got).Warn("safe context", "event_name", "context.forwarded")
					return want, nil
				})
				if streaming {
					edge = streamingCommandRunnerFunc{edge.(workerCommandRunnerFunc)}
				}
				runner := LoggingCommandRunner{Runner: edge, Logger: owner,
					Clock: &sequenceCommandClock{times: []time.Time{time.Unix(1, 0), time.Unix(3, 0)}}}
				var got CommandResult
				var err error
				if streaming {
					got, err = runner.RunStreaming(t.Context(), request, nil)
				} else {
					got, err = runner.Run(t.Context(), request)
				}
				if err != nil || !reflect.DeepEqual(got, want) || calls != 1 {
					t.Fatalf("result=%#v error=%v calls=%d", got, err, calls)
				}
				if runner.Logger != owner || (selection == "default" && request.ExecutionLogger != nil) {
					t.Fatal("caller or runner selection mutated")
				}
				assertSelectedCommandDiagnostics(t, selected, owner, override)
			})
		}
	}
}

func assertSelectedCommandDiagnostics(t *testing.T, selected logging.Logger, captures ...*recordingLogger) {
	t.Helper()
	for _, capture := range captures {
		if capture != selected {
			if len(capture.entries) != 0 {
				t.Fatalf("unselected logger received %#v", capture.entries)
			}
			continue
		}
		if len(capture.byEvent("command_runner.requested")) != 1 || len(capture.byEvent("context.forwarded")) != 1 {
			t.Fatalf("missing selected start/context: %#v", capture.entries)
		}
		completed := capture.byEventWithLevel("command_runner.completed")
		if len(completed) != 1 || completed[0].level != "info" || completed[0].fields["duration_ms"] != int64(2000) {
			t.Fatalf("terminal diagnostics = %#v", completed)
		}
		if len(capture.byEvent("command_runner.request_details")) != 1 || len(capture.byEvent("command_runner.output_details")) != 1 {
			t.Fatalf("verbose diagnostics = %#v", capture.entries)
		}
	}
}

func TestLoggingCommandRunnerConcurrentAttribution(t *testing.T) {
	t.Parallel()
	owner, override := &recordingLogger{}, &recordingLogger{}
	entered := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var wg sync.WaitGroup
	t.Cleanup(func() { unblock(); wg.Wait() })
	edge := workerCommandRunnerFunc(func(_ context.Context, request CommandRequest) (CommandResult, error) {
		entered <- request.DispatchID
		<-release
		commandContextLogger(logging.NoopLogger{}, request).Debug("safe context", "event_name", "context.forwarded")
		return CommandResult{Stdout: []byte(request.DispatchID)}, nil
	})
	runner := LoggingCommandRunner{Runner: edge, Logger: owner, Clock: ClockFunc(func() time.Time { return time.Unix(1, 0) })}
	requests := []CommandRequest{commandTestRequest(), commandTestRequest()}
	for i := range requests {
		suffix := fmt.Sprint(i)
		requests[i].DispatchID = "dispatch-" + suffix
		requests[i].TransitionID = "transition-" + suffix
		requests[i].WorkstationName = "station-" + suffix
		requests[i].Execution = work.ExecutionMetadata{RequestID: "request-" + suffix, TraceID: "trace-" + suffix, WorkIDs: []string{"work-" + suffix}}
	}
	requests[1].ExecutionLogger = override
	for _, request := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := runner.Run(t.Context(), request)
			if err == nil && string(result.Stdout) != request.DispatchID {
				err = fmt.Errorf("wrong peer result: %q", result.Stdout)
			}
			done <- err
		}()
	}
	for range requests {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatal("both requests did not enter controlled edge")
		}
	}
	unblock()
	for range requests {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("command did not finish")
		}
	}
	assertConcurrentCommandAttribution(t, requests, owner, override)
	if runner.Logger != owner || requests[0].ExecutionLogger != nil || requests[1].ExecutionLogger != override {
		t.Fatal("shared selection changed")
	}
}

func assertConcurrentCommandAttribution(t *testing.T, requests []CommandRequest, captures ...*recordingLogger) {
	t.Helper()
	for i, capture := range captures {
		if len(capture.byEvent("command_runner.requested")) != 1 || len(capture.byEvent("command_runner.completed")) != 1 {
			t.Fatalf("start/terminal count: %#v", capture.entries)
		}
		for _, event := range []string{"command_runner.requested", "command_runner.completed", "context.forwarded", "command_runner.output_details"} {
			fields := capture.byEvent(event)[0]
			request := requests[i]
			for key, want := range map[string]string{"request_id": request.Execution.RequestID, "trace_id": request.Execution.TraceID,
				"work_id": request.Execution.WorkIDs[0], "dispatch_id": request.DispatchID, "transition_id": request.TransitionID, "workstation_name": request.WorkstationName} {
				if fields[key] != want {
					t.Fatalf("%s %s=%v, want %s", event, key, fields[key], want)
				}
			}
		}
		if capture.byEvent("command_runner.completed")[0]["duration_ms"] != int64(0) {
			t.Fatal("fixed clock was not used")
		}
	}
}

func TestLoggingCommandRunnerAddsNoRawOutputDiagnostics(t *testing.T) {
	t.Parallel()
	capture := &recordingLogger{}
	request := commandTestRequest()
	request.Args = []string{"secret-args"}
	request.Stdin = []byte("secret-stdin")
	request.Env = []string{"TOKEN=secret-env"}
	wantErr := errors.New("secret-error")
	runner := LoggingCommandRunner{Logger: capture, Clock: ClockFunc(func() time.Time { return time.Unix(1, 0) }),
		Runner: workerCommandRunnerFunc(func(_ context.Context, got CommandRequest) (CommandResult, error) {
			commandContextLogger(logging.NoopLogger{}, got).Info("safe context", "event_name", "context.forwarded", "stdout_bytes", 13)
			return CommandResult{Stdout: []byte("secret-stdout"), Stderr: []byte("accepted tail"), ExitCode: 9}, wantErr
		})}
	_, err := runner.Run(t.Context(), request)
	assertCommandErrorIdentity(t, err, wantErr)
	for _, entry := range capture.entries {
		for key, value := range entry.fields {
			for _, secret := range []string{"secret-args", "secret-stdin", "secret-env", "secret-error", "secret-stdout"} {
				if strings.Contains(fmt.Sprint(value), secret) {
					t.Fatalf("%s exposed %s", key, secret)
				}
			}
			switch key {
			case "args", "stdin", "env", "stdout", "stderr", "error":
				t.Fatalf("new raw diagnostic key %s", key)
			}
		}
	}
	if capture.byEvent("command_runner.completed")[0]["stderr_tail"] != "accepted tail" {
		t.Fatal("operator-accepted stderr tail changed")
	}
}

func TestAdaptPlatformCommandRunnerPreservesSelectedContextLogger(t *testing.T) {
	t.Parallel()
	owner, override := &recordingLogger{}, &recordingLogger{}
	effect := platformprocess.ExecCommandRunner{Logger: owner}
	for _, platform := range []platformprocess.CommandRunner{effect, &effect} {
		adapted := AdaptPlatformCommandRunner(platform).(StreamingAdaptedCommandRunner)
		for _, selected := range []logging.Logger{owner, override, logging.NoopLogger{}} {
			request := commandTestRequest()
			if selected != owner {
				request.ExecutionLogger = selected
			}
			copied := platformCommandRunnerWithLogger(adapted.Runner, commandContextLogger(adapted.Logger, request))
			var logger logging.Logger
			switch typed := copied.(type) {
			case platformprocess.ExecCommandRunner:
				logger = typed.Logger
			case *platformprocess.ExecCommandRunner:
				logger = typed.Logger
			}
			logger.Info("safe cleanup", "event_name", "cleanup.forwarded")
			if effect.Logger != owner || adapted.Logger != owner {
				t.Fatal("stored platform or adapted logger mutated")
			}
		}
	}
	for _, capture := range []*recordingLogger{owner, override} {
		events := capture.byEvent("cleanup.forwarded")
		if len(events) != 2 || events[0]["dispatch_id"] != "dispatch-1" || events[0]["request_id"] != "request-1" {
			t.Fatalf("context forwarding = %#v", events)
		}
	}
	if commandContextLogger(nil, commandTestRequest()) != nil {
		t.Fatal("optional platform logger replaced")
	}
}

type mutatingCommandLogger struct {
	recordingLogger
	messages []string
}

func (logger *mutatingCommandLogger) mutate(level, message string, fields []any) {
	logger.messages = append(logger.messages, message)
	logger.record(level, fields)
	for i, value := range fields {
		if ids, ok := value.([]string); ok && len(ids) > 0 {
			ids[0] = "mutated-work"
		}
		fields[i] = "mutated-field"
	}
}
func (logger *mutatingCommandLogger) Debug(message string, fields ...any) {
	logger.mutate("debug", message, fields)
}
func (logger *mutatingCommandLogger) Info(message string, fields ...any) {
	logger.mutate("info", message, fields)
}
func (logger *mutatingCommandLogger) Warn(message string, fields ...any) {
	logger.mutate("warn", message, fields)
}
func (logger *mutatingCommandLogger) Error(message string, fields ...any) {
	logger.mutate("error", message, fields)
}
func (logger *mutatingCommandLogger) Verbose(message string, fields ...any) {
	logger.mutate("verbose", message, fields)
}

func TestCommandContextLoggerPreservesIsolatedForwarding(t *testing.T) {
	t.Parallel()
	selected := &mutatingCommandLogger{}
	request := commandTestRequest()
	request.ExecutionLogger = selected
	logger := commandContextLogger(logging.NoopLogger{}, request)
	values := []any{"event_name", "context.forwarded"}
	for _, emit := range []func(string, ...any){logger.Debug, logger.Info, logger.Warn, logger.Error, logger.Verbose} {
		emit("unchanged message", values...)
	}
	if !reflect.DeepEqual(values, []any{"event_name", "context.forwarded"}) || request.Execution.WorkIDs[0] != "work-1" {
		t.Fatal("caller fields mutated")
	}
	for i, entry := range selected.entries {
		if entry.level != []string{"debug", "info", "warn", "error", "verbose"}[i] || selected.messages[i] != "unchanged message" || entry.fields["work_id"] != "work-1" ||
			!reflect.DeepEqual(entry.fields["work_ids"], []string{"work-1"}) {
			t.Fatalf("forwarded record=%#v message=%q", entry, selected.messages[i])
		}
	}
	if logger.(contextualLogger).fields[9] != "dispatch-1" {
		t.Fatal("retained context fields mutated")
	}
}

func TestCloneCommandRequestPreservesCallerState(t *testing.T) {
	t.Parallel()
	request := commandTestRequest()
	request.PreviousChainingTraceIDs = []string{"previous"}
	request.Inputs = []workers.WorkInput{{WorkID: "work-1", Tags: map[string]string{"tag": "value"}, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "text"}}}}
	cloned := CloneCommandRequest(request)
	cloned.Args[0], cloned.Env[0], cloned.Execution.WorkIDs[0], cloned.PreviousChainingTraceIDs[0] = "changed", "changed", "changed", "changed"
	cloned.Stdin[0] = 'X'
	cloned.Inputs[0].Tags["tag"] = "changed"
	cloned.Inputs[0].Content[0].Text = "changed"
	if request.Args[0] != "--fixture" || request.Env[0] != "VISIBLE=1" || string(request.Stdin) != "input" || request.Execution.WorkIDs[0] != "work-1" ||
		request.PreviousChainingTraceIDs[0] != "previous" || request.Inputs[0].Tags["tag"] != "value" || request.Inputs[0].Content[0].Text != "text" {
		t.Fatal("clone changed caller state")
	}
	platform := platformRequest(request)
	platform.Args[0], platform.Env[0], platform.Stdin[0] = "platform-change", "platform-change", 'Y'
	private := workerRequest(platform)
	private.Args[0], private.Env[0], private.Stdin[0] = "private-change", "private-change", 'Z'
	if request.Args[0] != "--fixture" || string(request.Stdin) != "input" || platform.Args[0] != "platform-change" || platform.Stdin[0] != 'Y' {
		t.Fatal("projection aliases caller buffers")
	}
}

type commandLifecycleRecorder struct{ started, exited []int }

func (observer *commandLifecycleRecorder) ProcessStarted(info platformprocess.ProcessInfo) {
	observer.started = append(observer.started, info.PID)
}
func (observer *commandLifecycleRecorder) ProcessExited(info platformprocess.ProcessInfo) {
	observer.exited = append(observer.exited, info.PID)
}

type lifecycleCommandEffect struct {
	request platformprocess.CommandRequest
	calls   int
}

func (effect *lifecycleCommandEffect) Run(_ context.Context, request platformprocess.CommandRequest) (CommandResult, error) {
	effect.request = request
	effect.calls++
	request.ProcessLifecycleObserver.ProcessStarted(platformprocess.ProcessInfo{PID: 42})
	request.ProcessLifecycleObserver.ProcessExited(platformprocess.ProcessInfo{PID: 42})
	request.Args[0], request.Env[0], request.Stdin[0] = "effect-change", "effect-change", 'X'
	return CommandResult{Stdout: []byte("out"), Stderr: []byte("err")}, nil
}

func TestAdaptCommandRunnerPreservesLifecycleAndProjectionIsolation(t *testing.T) {
	t.Parallel()
	effect := &lifecycleCommandEffect{}
	observer := &commandLifecycleRecorder{}
	request := commandTestRequest()
	request.ProcessLifecycleObserver = observer
	request.ExecutionLogger = logging.NoopLogger{}
	runner := AdaptPlatformCommandRunner(effect)
	result, err := runner.Run(t.Context(), request)
	if err != nil || string(result.Stdout) != "out" || effect.calls != 1 {
		t.Fatalf("result=%#v error=%v calls=%d", result, err, effect.calls)
	}
	if effect.request.ProcessLifecycleObserver != observer || effect.request.ExecutionLogger != request.ExecutionLogger ||
		!reflect.DeepEqual(observer.started, []int{42}) || !reflect.DeepEqual(observer.exited, []int{42}) {
		t.Fatal("lifecycle or logger forwarding changed")
	}
	if request.Args[0] != "--fixture" || request.Env[0] != "VISIBLE=1" || string(request.Stdin) != "input" {
		t.Fatal("effect mutated caller request")
	}
	assertProjectedCommandLifecycleAndFallback(t, request, result, observer)
}

func assertProjectedCommandLifecycleAndFallback(t *testing.T, request CommandRequest, result CommandResult, observer *commandLifecycleRecorder) {
	t.Helper()
	// A private runner projected to the platform boundary keeps buffered fallback
	// output ordered and copied, without inventing a second command execution.
	calls := 0
	private := workerCommandRunnerFunc(func(_ context.Context, got CommandRequest) (CommandResult, error) {
		calls++
		if got.ProcessLifecycleObserver != observer {
			t.Error("projected lifecycle observer changed")
		}
		got.Args[0] = "private-change"
		return result, nil
	})
	projected := ProjectPlatformCommandRunner(private).(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, OutputChunkObserver) (CommandResult, error)
	})
	platform := platformRequest(request)
	var chunks []string
	got, err := projected.RunStreaming(t.Context(), platform, func(stream string, chunk []byte) {
		chunks = append(chunks, stream+":"+string(chunk))
		chunk[0] = 'X'
	})
	if err != nil || !reflect.DeepEqual(got, result) || calls != 1 || !reflect.DeepEqual(chunks, []string{"stdout:out", "stderr:err"}) || platform.Args[0] != "--fixture" {
		t.Fatalf("projected result=%#v error=%v calls=%d chunks=%v", got, err, calls, chunks)
	}
}

func TestStreamingAdaptedCommandRunnerPreservesOrderAndNilObserver(t *testing.T) {
	t.Parallel()
	effect := &streamingRecordingEffect{}
	runner := AdaptPlatformCommandRunner(effect).(StreamingCommandRunner)
	var chunks []string
	result, err := runner.RunStreaming(t.Context(), commandTestRequest(), func(stream string, chunk []byte) {
		chunks = append(chunks, stream+":"+string(chunk))
		chunk[0] = 'X'
	})
	if err != nil || !reflect.DeepEqual(chunks, []string{"stderr:warn", "stdout:ok"}) || string(result.Stdout) != "ok" || string(result.Stderr) != "warn" || len(effect.chunks) != 1 {
		t.Fatalf("result=%#v error=%v chunks=%v calls=%v", result, err, chunks, effect.chunks)
	}
	got, err := runner.RunStreaming(t.Context(), commandTestRequest(), nil)
	if err != nil || !reflect.DeepEqual(got, result) || len(effect.chunks) != 2 {
		t.Fatalf("nil observer result=%#v error=%v", got, err)
	}
}
