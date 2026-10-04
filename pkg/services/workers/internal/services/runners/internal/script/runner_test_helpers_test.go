package script

import (
	"context"
	"errors"
	"reflect"
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
	mu        sync.Mutex
	values    []string
	terminal  workers.ScriptResponseEventPayload
	fragments []workers.ProgressFragment
	events    []workers.ScriptEvent
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

// Capture before callbacks mutate their arguments, so assertions see emitted facts.
func (log *observationLog) CaptureProgress(fragment workers.ProgressFragment) {
	log.mu.Lock()
	defer log.mu.Unlock()
	fragment.Metadata = cloneStringMap(fragment.Metadata)
	log.fragments = append(log.fragments, fragment)
}

func (log *observationLog) CaptureEvent(event workers.ScriptEvent) {
	log.mu.Lock()
	defer log.mu.Unlock()
	event.TraceIDs = append([]string(nil), event.TraceIDs...)
	event.WorkIDs = append([]string(nil), event.WorkIDs...)
	if event.Request != nil {
		payload := *event.Request
		payload.Args = append([]string(nil), payload.Args...)
		event.Request = &payload
	}
	if event.Response != nil {
		payload := *event.Response
		if payload.ExitCode != nil {
			code := *payload.ExitCode
			payload.ExitCode = &code
		}
		if payload.FailureType != nil {
			failure := *payload.FailureType
			payload.FailureType = &failure
		}
		event.Response = &payload
	}
	log.events = append(log.events, event)
}

func observedRequest() workers.RunnerExecutionRequest {
	request := validRequest()
	request.Dispatch.Execution.CurrentTick = 23
	request.Dispatch.Execution.DispatchCreatedTick = 19
	request.Correlation = workers.ExecutionCorrelation{
		FactorySessionID: "session-1", RuntimeID: "runtime-1", GenerationID: "generation-1",
		DispatchID: "dispatch-1", AttemptID: "attempt-1", RequestID: "request-1", TraceID: "trace-1",
	}
	return request
}

func assertObservedValue(t *testing.T, field string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}

func assertAttributedObservations(t *testing.T, log *observationLog, request workers.RunnerExecutionRequest,
	started, finished time.Time, command string, args []string, chunks []outputChunk) {
	t.Helper()
	log.mu.Lock()
	defer log.mu.Unlock()
	assertObservedValue(t, "progress count", len(log.fragments), len(chunks))
	for i, fragment := range log.fragments {
		assertObservedValue(t, "progress kind", fragment.Kind, workers.ProgressFragmentKind)
		assertObservedValue(t, "progress dispatch", fragment.DispatchID, "dispatch-1")
		assertObservedValue(t, "progress correlation", fragment.Correlation, request.Correlation)
		assertObservedValue(t, "progress stream", fragment.Type, chunks[i].stream)
		assertObservedValue(t, "progress payload", fragment.Payload, chunks[i].payload)
		assertObservedValue(t, "progress metadata", fragment.Metadata, map[string]string{"stream": chunks[i].stream})
	}
	assertObservedValue(t, "event count", len(log.events), 2)
	for i, event := range log.events {
		assertObservedValue(t, "event dispatch", event.DispatchID, "dispatch-1")
		assertObservedValue(t, "event request", event.RequestID, "request-1")
		assertObservedValue(t, "event trace", event.TraceIDs, []string{"trace-1"})
		assertObservedValue(t, "event work", event.WorkIDs, []string{"work-1"})
		assertObservedValue(t, "event tick", event.Tick, 23)
		assertObservedValue(t, "event UTC location", event.EventTime.Location(), time.UTC)
		if i == 0 {
			assertObservedValue(t, "request event ID", event.ID, "factory-event/script-request/dispatch-1/script-request/1")
			assertObservedValue(t, "request event kind", event.Kind, workers.ScriptEventKindRequest)
			assertObservedValue(t, "request time", event.EventTime, started.UTC())
			assertObservedValue(t, "request response payload", event.Response, (*workers.ScriptResponseEventPayload)(nil))
			if event.Request == nil {
				t.Fatal("request payload missing")
			}
			assertObservedValue(t, "request args", event.Request.Args, args)
			assertObservedValue(t, "request attempt", event.Request.Attempt, 1)
			assertObservedValue(t, "request command", event.Request.Command, command)
			assertObservedValue(t, "request dispatch", event.Request.DispatchID, "dispatch-1")
			assertObservedValue(t, "request script request", event.Request.ScriptRequestID, "dispatch-1/script-request/1")
			assertObservedValue(t, "request transition", event.Request.TransitionID, "transition-1")
		} else {
			assertObservedValue(t, "response event ID", event.ID, "factory-event/script-response/dispatch-1/1")
			assertObservedValue(t, "response event kind", event.Kind, workers.ScriptEventKindResponse)
			assertObservedValue(t, "response time", event.EventTime, finished.UTC())
			assertObservedValue(t, "response request payload", event.Request, (*workers.ScriptRequestEventPayload)(nil))
			if event.Response == nil {
				t.Fatal("response payload missing")
			}
			assertObservedValue(t, "response attempt", event.Response.Attempt, 1)
			assertObservedValue(t, "response dispatch", event.Response.DispatchID, "dispatch-1")
			assertObservedValue(t, "response script request", event.Response.ScriptRequestID, "dispatch-1/script-request/1")
			assertObservedValue(t, "response transition", event.Response.TransitionID, "transition-1")
		}
	}
}

func newTestRunner(
	t *testing.T,
	config Config,
	commandRunner workerprocess.StreamingCommandRunner,
) workers.Runner {
	t.Helper()
	return New(config, commandRunner, emptyDocs,
		func() time.Time { return time.Unix(0, 0) },
		func(workers.ProgressFragment) {},
		func(workers.ScriptEvent) {},
	)
}

func assertDefaultExecutionAfterOverride(t *testing.T, scriptRunner workers.Runner,
	commandEdge *streamingCommandEdge, constructionLog, overrideLog *observationLog,
	overrideRequest workers.RunnerExecutionRequest, overridden workers.RunnerExecutionResult,
	started time.Time, constructionChunks, overrideChunks []outputChunk) {
	t.Helper()
	overridden.Diagnostics.Command.Args[0] = "override mutation"
	overridden.Diagnostics.Command.Env["RUNTIME"] = "override mutation"
	overridden.Diagnostics.Metadata["dispatch_id"] = "override mutation"
	request := observedRequest()
	request.Correlation.RuntimeID = "default-runtime"
	request.Correlation.AttemptID = "default-attempt"
	result, err := scriptRunner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("default Execute() after override error = %v", err)
	}
	assertObservedValue(t, "default output", result.Content, "construction")
	assertObservedValue(t, "default command calls", constructionLog.Values(), []string{"command"})
	assertObservedValue(t, "default selected command", commandEdge.Request().Command, "echo")
	assertObservedValue(t, "default command dispatch", commandEdge.Request().DispatchID, "dispatch-1")
	assertObservedValue(t, "default detached args", result.Diagnostics.Command.Args, []string{"safe-arg"})
	assertObservedValue(t, "default detached environment", result.Diagnostics.Command.Env["RUNTIME"], "<metadata-only>")
	assertObservedValue(t, "default detached metadata", result.Diagnostics.Metadata["dispatch_id"], "dispatch-1")
	assertAttributedObservations(t, constructionLog, request, started.Add(2*time.Second),
		started.Add(3*time.Second), "echo", []string{"safe-arg"}, constructionChunks)
	assertObservedValue(t, "default terminal stdout", constructionLog.events[1].Response.Stdout, "construction")
	assertObservedValue(t, "default terminal stderr", constructionLog.events[1].Response.Stderr, "construction note")
	assertObservedValue(t, "default terminal outcome", constructionLog.events[1].Response.Outcome, workers.ScriptExecutionOutcomeSucceeded)
	// The default execution must not publish or record into the request-local captures.
	assertAttributedObservations(t, overrideLog, overrideRequest, started, started.Add(time.Second),
		"echo", []string{"safe-arg"}, overrideChunks)
	assertObservedValue(t, "override terminal stdout", overrideLog.events[1].Response.Stdout, overrideChunks[0].payload)
	assertObservedValue(t, "override terminal outcome", overrideLog.events[1].Response.Outcome, workers.ScriptExecutionOutcomeSucceeded)
	result.Diagnostics.Command.Args[0] = "default mutation"
	result.Diagnostics.Command.Env["RUNTIME"] = "default mutation"
	result.Diagnostics.Metadata["dispatch_id"] = "default mutation"
	assertObservedValue(t, "override output stays detached", overridden.Content, overrideChunks[0].payload)
	assertObservedValue(t, "override args stay detached", overridden.Diagnostics.Command.Args, []string{"override mutation"})
	assertObservedValue(t, "override environment stays detached", overridden.Diagnostics.Command.Env["RUNTIME"], "override mutation")
	assertObservedValue(t, "override metadata stays detached", overridden.Diagnostics.Metadata["dispatch_id"], "override mutation")
}
