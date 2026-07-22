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
}

func (r *recordingEffect) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.request = request
	return platformprocess.CommandResult{Stdout: []byte("ok")}, nil
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
