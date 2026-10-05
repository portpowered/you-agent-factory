package customer_commands_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformlogging "github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The cohort starts on the parent goroutine rather than parallel child bodies:
// all five effects must enter even with go test -parallel 1. No gate holds a
// shared invocation lock; each invocation owns its session, profile and streams.
func testOutputConcurrentQuietAndVerboseInvocationsKeepOwnFraming(t *testing.T) {
	t.Parallel()
	selections := [][]string{
		{"--quiet"}, {"--json", "--output", "primary"},
		{"--json", "--output", "response-stream"}, {}, {"--verbose", "--debug"},
	}
	var cohort []*concurrentOutputCall
	for index, flags := range selections {
		call := newConcurrentOutputCall(t, index+1, flags)
		cohort = append(cohort, call)
	}
	for _, call := range cohort {
		call.start()
	}
	for _, call := range cohort {
		select {
		case <-call.runner.entered:
		case <-call.done:
			t.Fatalf("%s completed before worker entry: %v", call.marker, call.err)
		case <-call.ctx.Done():
			t.Fatalf("%s worker entry: %v", call.marker, call.ctx.Err())
		}
	}
	for _, call := range cohort {
		select {
		case <-call.done:
			t.Fatalf("%s completed before cohort release", call.marker)
		default:
		}
	}
	t.Log("all five owned provider effects entered before any release; all invocations remain live")
	for _, call := range cohort {
		call.runner.release()
	}
	for _, call := range cohort {
		call.join(t)
	}
	for index, call := range cohort {
		t.Run(call.marker, func(t *testing.T) {
			t.Parallel()
			assertConcurrentOutputSuccess(t, index, call, cohort)
		})
	}
	for index, flags := range [][]string{{"--quiet", "--json"}, {"--quiet", "--output", "primary"}} {
		t.Run([]string{"F14-C06", "F14-C07"}[index], func(t *testing.T) {
			t.Parallel()
			assertConcurrentOutputConflict(t, index+6, flags)
		})
	}
}

type concurrentOutputCall struct {
	fixture    *machineOutputFixture
	inputs     *support.CapturedInputs
	sessionID  string
	factoryDir string
	marker     string
	runner     *concurrentOutputRunner
	ctx        context.Context
	done       chan struct{}
	started    bool
	err        error
}

func newConcurrentOutputCall(t *testing.T, number int, flags []string) *concurrentOutputCall {
	t.Helper()
	return newConcurrentOutputCallWithProviderOverride(t, number, flags, true)
}

func newConcurrentOutputCallWithProviderOverride(t *testing.T, number int, flags []string, override bool) *concurrentOutputCall {
	t.Helper()
	marker := fmt.Sprintf("F14-C%02d", number)
	args := append([]string{"you", "run"}, flags...)
	args = append(args, "--named", goalFactoryName)
	if override {
		args = append(args, "--executor-provider", "codex")
	}
	args = append(args, "--executor-model", "gpt-5-codex", "--no-record", "owned input "+marker)
	fixture, inputs, factoryDir := newMachineOutputInputs(t, args, goalFactoryName)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	inputs.Input.Context = ctx
	runner := &concurrentOutputRunner{sessionID: sessionID,
		entered: make(chan struct{}), gate: make(chan struct{})}
	payload, err := json.Marshal(map[string]string{"decision": "accepted", "feedback": "", "output": marker})
	if err != nil {
		t.Fatal(err)
	}
	runner.delegate = support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: payload})
	route := fixture.router.bind(sessionID, runner)
	inputs.Input.Args = append([]string{"you", "--remote", "--server", fixture.baseURL,
		"run", "--session", sessionID}, args[2:]...)
	call := &concurrentOutputCall{fixture: fixture, inputs: inputs, sessionID: sessionID, factoryDir: factoryDir,
		marker: marker, runner: runner, ctx: ctx, done: make(chan struct{})}
	t.Cleanup(func() {
		runner.release()
		cancel()
		if call.started {
			call.join(t)
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unbind(route)
	})
	return call
}

func (call *concurrentOutputCall) start() {
	call.started = true
	go func() {
		call.err = call.fixture.process.Execute(call.inputs.Input)
		close(call.done)
	}()
}

func (call *concurrentOutputCall) join(t *testing.T) {
	t.Helper()
	select {
	case <-call.done:
	case <-time.After(2 * time.Minute):
		t.Fatalf("%s invocation did not join", call.marker)
	}
}

type concurrentOutputRunner struct {
	sessionID              string
	entered, gate          chan struct{}
	entryOnce, releaseOnce sync.Once
	calls                  atomic.Int64
	delegate               platformprocess.CommandRunner
	command                atomic.Value
}

func (runner *concurrentOutputRunner) release() {
	runner.releaseOnce.Do(func() { close(runner.gate) })
}

func (runner *concurrentOutputRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	if request.ExecutionScopeID != runner.sessionID {
		return platformprocess.CommandResult{}, errors.New("provider effect entered peer session")
	}
	runner.command.Store(request.Command)
	runner.entryOnce.Do(func() { close(runner.entered) })
	select {
	case <-runner.gate:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return runner.delegate.Run(ctx, request)
}

func assertConcurrentOutputSuccess(t *testing.T, index int, call *concurrentOutputCall, peers []*concurrentOutputCall) {
	t.Helper()
	stdout, stderr := call.inputs.Stdout(), call.inputs.Stderr()
	t.Logf("controlled CLI session=%s executionScopeID=%s error=%v stdout=%q stderr=%q", call.sessionID, call.runner.sessionID, call.err, stdout, stderr)
	if call.err != nil || stderr != "" {
		t.Fatalf("success error=%v stderr=%q", call.err, stderr)
	}
	for _, peer := range peers {
		if peer != call && strings.Contains(stdout+stderr, peer.marker) {
			t.Fatalf("peer result %s leaked into %s", peer.marker, call.marker)
		}
	}
	switch index {
	case 0:
		if stdout != call.marker {
			t.Fatalf("quiet stdout=%q, want only raw result %q", stdout, call.marker)
		}
	case 1:
		decoder := json.NewDecoder(strings.NewReader(stdout))
		var response factoryapi.InvocationResponse
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			t.Fatalf("data after single JSON response: %v", err)
		}
		assertConcurrentOutputResult(t, call, response)
		assertNoPrivateRuntimeKeysInJSON(t, stdout, "stdout")
	case 2:
		assertConcurrentOutputNDJSON(t, call, stdout)
	default:
		assertConcurrentOutputHuman(t, call, stdout)
	}
	events := support.GetFactoryEventsForSessionAt(t, call.fixture.baseURL, call.sessionID)
	if support.CountFactoryEvents(events, factoryapi.FactoryEventTypeWorkRequest) == 0 || support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("owned public admission/dispatch not observed")
	}
}

func assertConcurrentOutputHuman(t *testing.T, call *concurrentOutputCall, stdout string) {
	t.Helper()
	assertHumanStdoutFreeOfStructuredEnvelopeNoise(t, stdout)
	lines := nonEmptyStdoutLines(stdout)
	if len(lines) < 3 || lines[len(lines)-2] != "--- primary result ---" || lines[len(lines)-1] != call.marker {
		t.Fatalf("human result framing=%q", stdout)
	}
	for _, line := range lines[:len(lines)-2] {
		if !isHumanFactoryLifecycleLine(line) {
			t.Fatalf("unexpected human lifecycle line: %q", line)
		}
	}
	previous := -1
	for _, prefix := range []string{"factory started", "work accepted:", "workstation started:", "workstation completed:"} {
		position := strings.Index(stdout, prefix)
		if position <= previous {
			t.Fatalf("human lifecycle is missing or out of order at %q: %q", prefix, stdout)
		}
		previous = position
	}
}

func assertConcurrentOutputResult(t *testing.T, call *concurrentOutputCall, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || invocationPrimaryResultText(t, response) != call.marker {
		t.Fatalf("terminal result=%#v", response)
	}
	if response.SessionId == nil || *response.SessionId != call.sessionID {
		t.Fatalf("terminal session=%v, want %s", response.SessionId, call.sessionID)
	}
}

func assertConcurrentOutputNDJSON(t *testing.T, call *concurrentOutputCall, stdout string) {
	t.Helper()
	records := decodeNDJSONRecords(t, stdout)
	if len(records) < 2 {
		t.Fatal("want public events followed by terminal result")
	}
	sequence, sessionSequence := -1, -1
	for index, record := range records {
		if index == len(records)-1 {
			if record.RecordType != invocationResultType {
				t.Fatal("last record is not invocation_result")
			}
			var response factoryapi.InvocationResponse
			if err := json.Unmarshal(record.Payload, &response); err != nil {
				t.Fatal(err)
			}
			assertConcurrentOutputResult(t, call, response)
			assertNoPrivateRuntimeKeysInJSON(t, string(record.Payload), "terminal result")
			continue
		}
		if record.RecordType != factoryEventRecordType {
			t.Fatalf("nonterminal record type=%s", record.RecordType)
		}
		assertFactoryEventRecord(t, record, index)
		assertFactoryEventSequenceMonotonic(t, record, index, &sequence, &sessionSequence)
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			t.Fatal(err)
		}
		switch index {
		case 0, 1, 3:
			assertConcurrentOutputStartup(t, call, event, index)
		default:
			if event.Context.SessionId == nil || *event.Context.SessionId != call.sessionID {
				t.Fatalf("event belongs to peer session: %#v", event.Context)
			}
			if index == 2 && (event.Type != factoryapi.FactoryEventTypeSessionStarted || event.Id != "factory-event/session-started" || event.Context.Sequence != 2) {
				t.Fatalf("owned session start frame=%#v", event)
			}
		}
	}
	if sessionSequence < 0 {
		t.Fatal("no public session sequence observed")
	}
}

func assertConcurrentOutputStartup(t *testing.T, call *concurrentOutputCall, event factoryapi.FactoryEvent, index int) {
	t.Helper()
	// Binding operator decisions 2026-10-03T10:44Z and 10:50Z: characterize
	// exactly the current three sessionless startup frames. A later policy fix
	// must update this proof; no other sessionless frame is permitted.
	if event.Context.SessionId != nil || event.Context.Sequence != index {
		t.Fatalf("current startup context=%#v, want sessionless sequence %d", event.Context, index)
	}
	var factory factoryapi.Factory
	switch index {
	case 0:
		if event.Id != "factory-event/run-started" || event.Type != factoryapi.FactoryEventTypeRunRequest {
			t.Fatalf("run-started frame=%#v", event)
		}
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		factory = payload.Factory
	case 1:
		if event.Id != "factory-event/initial-structure/0" || event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			t.Fatalf("initial structure frame=%#v", event)
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		factory = payload.Factory
	case 3:
		if event.Id != "factory-event/factory-state-change/0/RUNNING" || event.Type != factoryapi.FactoryEventTypeFactoryStateResponse {
			t.Fatalf("initial running-state frame=%#v", event)
		}
		payload, err := event.Payload.AsFactoryStateResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if payload.PreviousState == nil || *payload.PreviousState != factoryapi.FactoryStateIdle || payload.State != factoryapi.FactoryStateRunning || payload.Reason == nil || *payload.Reason != "run started" {
			t.Fatalf("initial running-state payload=%#v", payload)
		}
		return
	}
	if factory.FactoryDirectory == nil || filepath.Clean(*factory.FactoryDirectory) != filepath.Clean(call.factoryDir) {
		t.Fatalf("startup Factory directory=%v, want owned directory %s", factory.FactoryDirectory, call.factoryDir)
	}
}

func assertConcurrentOutputConflict(t *testing.T, number int, flags []string) {
	t.Helper()
	call := newConcurrentOutputCall(t, number, flags)
	call.start()
	call.join(t)
	stdout, stderr := call.inputs.Stdout(), call.inputs.Stderr()
	t.Logf("controlled CLI session=%s error=%v stdout=%q stderr=%q", call.sessionID, call.err, stdout, stderr)
	var invocationError *runcli.InvocationError
	if !errors.As(call.err, &invocationError) || invocationError.Code != runcli.InvocationOutputConflictCode {
		t.Fatalf("error=%v, want typed output conflict", call.err)
	}
	response := decodeSingleJSONErrorResponse(t, stderr)
	if stdout != "" || response.Code != runcli.InvocationOutputConflictCode || response.Family != factoryapi.ErrorFamilyBadRequest || response.Message != "--quiet cannot be used with --json or --output" {
		t.Fatalf("conflict stdout=%q response=%#v", stdout, response)
	}
	if call.runner.calls.Load() != 0 {
		t.Fatal("conflict dispatched provider effect")
	}
	for _, event := range support.GetFactoryEventsForSessionAt(t, call.fixture.baseURL, call.sessionID) {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest || event.Type == factoryapi.FactoryEventTypeDispatchRequest {
			t.Fatalf("conflict admitted/dispatched: %#v", event)
		}
	}
}

// The same public CLI output policy applies to selected terminal classifications.
// Both invocations enter before either is released, sharing the package host
// while retaining explicit sessions and scenario-owned provider routes.
func testOutputInjectedInvocationSelectedEffectsAndOutputPolicy(t *testing.T) {
	t.Parallel()
	for round, policy := range []struct{ tty, override bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		t.Run(fmt.Sprintf("tty=%t/providerOverride=%t", policy.tty, policy.override), func(t *testing.T) {
			tty := policy.tty
			quiet := newConcurrentOutputCallWithProviderOverride(t, 20+round*2, []string{"--quiet"}, policy.override)
			normal := newConcurrentOutputCallWithProviderOverride(t, 21+round*2, nil, policy.override)
			for _, call := range []*concurrentOutputCall{quiet, normal} {
				call.inputs.Input.StdoutIsTTY = &tty
				call.inputs.Input.StderrIsTTY = &tty
				call.start()
			}
			for _, call := range []*concurrentOutputCall{quiet, normal} {
				select {
				case <-call.runner.entered:
				case <-call.done:
					t.Fatalf("effect returned before entry: %v", call.err)
				case <-call.ctx.Done():
					t.Fatal(call.ctx.Err())
				}
			}
			quiet.runner.release()
			normal.runner.release()
			quiet.join(t)
			normal.join(t)
			for _, call := range []*concurrentOutputCall{quiet, normal} {
				if command := call.runner.command.Load(); command != "codex" {
					t.Fatalf("selected provider command = %q, want codex", command)
				}
			}
			assertInjectedOutputDiagnostics(t, quiet)
			assertInjectedOutputDiagnostics(t, normal)
			assertConcurrentOutputSuccess(t, 0, quiet, []*concurrentOutputCall{quiet, normal})
			if !tty {
				assertConcurrentOutputSuccess(t, 3, normal, []*concurrentOutputCall{quiet, normal})
			} else {
				if normal.err != nil {
					t.Fatal(normal.err)
				}
				stdout, stderr := normal.inputs.Stdout(), normal.inputs.Stderr()
				if !strings.Contains(stdout, "\x1b[") || !strings.Contains(stderr, "\r\x1b[2K") || !strings.Contains(stderr, "execute-goal") {
					t.Fatalf("TTY lifecycle/progress routing: stdout=%q stderr=%q", stdout, stderr)
				}
				if strings.Contains(stdout+stderr, quiet.marker) || strings.Contains(stderr, normal.marker) {
					t.Fatal("TTY output leaked peer or routed result to progress")
				}
				plain := regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]").ReplaceAllString(stdout, "")
				assertConcurrentOutputHuman(t, normal, plain)
			}
		})
	}
}

func assertInjectedOutputDiagnostics(t *testing.T, call *concurrentOutputCall) {
	t.Helper()
	requestID, traceID := "", ""
	submitted, completed := 0, 0
	for _, entry := range call.fixture.diagnostics.All() {
		fields := entry.ContextMap()
		if fields["session_id"] != call.sessionID || !strings.HasPrefix(entry.Message, "factory session invocation ") {
			continue
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "owned input "+call.marker) {
			t.Fatal("invocation diagnostic leaked caller input")
		}
		switch entry.Message {
		case "factory session invocation submitted":
			submitted++
			requestID, _ = fields["request_id"].(string)
			traceID, _ = fields["trace_id"].(string)
		case "factory session invocation completed":
			completed++
			assertInjectedCompletedDiagnostic(t, fields, requestID, traceID)
		}
	}
	if submitted != 1 || completed != 1 || requestID == "" || traceID == "" {
		t.Fatalf("CLI session %s submitted/completed=%d/%d request=%s trace=%s", call.sessionID, submitted, completed, requestID, traceID)
	}
}

type selectedRunWall struct{ nanos atomic.Int64 }

func (source *selectedRunWall) Now() time.Time { return time.Unix(0, source.nanos.Load()).UTC() }

type selectedRunTimeRunner struct {
	source      *selectedRunWall
	completedAt time.Time
}

func (runner selectedRunTimeRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.source.nanos.Store(runner.completedAt.UnixNano())
	return support.NewStaticSuccessCommandRunner("selected time COMPLETE").Run(ctx, request)
}

func testOutputSelectedProcessClockDatesRecordingAndCLIRunFacts(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit=%t", explicit), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := support.ScaffoldFactory(t, textStreamPromptRunFactoryConfig())
			support.WriteAgentConfig(t, dir, "mock-worker", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
			base := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
			source := &selectedRunWall{}
			source.nanos.Store(base.UnixNano())
			var recordingService recordings.Service
			recordingID := recordings.RecordingID("019a07c0-5000-7000-8000-000000000001")
			process := support.BuildProcess(t, serviceedges.Edges{Clock: source,
				ProviderCommandRunner:                    selectedRunTimeRunner{source: source, completedAt: base.Add(7 * time.Second)},
				FactorySessionRuntimeInstanceIDGenerator: func() string { return string(recordingID) },
				RecordingsRootObserver:                   func(service recordings.Service) { recordingService = service },
			})
			args := []string{"you", "run", "--factory", dir, "--verbose", "--debug"}
			destination := filepath.Join(home, "explicit.recording.json")
			if explicit {
				args = append(args, "--record", destination)
			}
			requestPath := filepath.Join(home, "work.json")
			request, err := json.Marshal(work.WorkRequest{RequestID: "selected-time-request", Type: work.WorkRequestTypeFactoryRequestBatch, Works: []work.Work{{WorkID: "selected-time-work", Name: "selected-time", WorkTypeID: textStreamPromptRunWorkType, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "selected time input"}}}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(requestPath, request, 0o600); err != nil {
				t.Fatal(err)
			}
			args = append(args, "--work", requestPath)
			input := support.FakeInputs(t.Context(), args)
			input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			input.WorkingDirectory = dir
			if err := process.Execute(input.Input); err != nil {
				t.Fatalf("run: %v; stderr=%s", err, input.Stderr())
			}
			if input.Stdout() != "selected time COMPLETE" {
				t.Fatalf("clean output = %q", input.Stdout())
			}
			if !explicit {
				paths, err := filepath.Glob(filepath.Join(home, ".you-agent-factory", "recordings", "2041", "02", "03", "*.json"))
				if err != nil || len(paths) != 1 {
					t.Fatalf("selected dated recording = %v, %v", paths, err)
				}
				destination = paths[0]
				if _, err := uuid.Parse(strings.TrimSuffix(filepath.Base(destination), ".json")); err != nil {
					t.Fatalf("recording UUID: %v", err)
				}
			}
			if _, err := os.Stat(destination); err != nil {
				t.Fatalf("recording destination: %v", err)
			}
			assertSelectedRunArtifacts(t, home, base)
			assertSelectedRunRecordingFacts(t, recordingService, recordingID, source, base)
		})
	}
}

func assertSelectedRunArtifacts(t *testing.T, home string, openedAt time.Time) {
	t.Helper()
	for _, root := range []string{platformlogging.RuntimeLogsRoot(home), platformmetrics.RuntimeMetricsRoot(home)} {
		pattern := filepath.Join(root, openedAt.Format("2006"), openedAt.Format("01"), openedAt.Format("02"), openedAt.Format("150405.000000000")+"-*")
		paths, err := filepath.Glob(pattern)
		if err != nil || len(paths) == 0 {
			t.Fatalf("selected-time artifact %q: %v, %v", pattern, paths, err)
		}
	}
}

func assertSelectedRunRecordingFacts(t *testing.T, service recordings.Service, id recordings.RecordingID, source *selectedRunWall, base time.Time) {
	t.Helper()
	facts, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{RecordingID: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Recording.Events) == 0 || !facts.Recording.Events[0].RecordedAt.Equal(base) {
		t.Fatalf("initial recorded facts = %#v", facts.Recording)
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
	if err != nil || status.Status.FinalizedAt == nil || !status.Status.FinalizedAt.Equal(base.Add(7*time.Second)) {
		t.Fatalf("finalized facts = %#v, %v", status, err)
	}
	source.nanos.Store(base.Add(time.Hour).UnixNano())
	replay, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{RecordingID: id})
	if err != nil || !reflect.DeepEqual(replay.Recording.Events, facts.Recording.Events) {
		t.Fatalf("selected source advance changed recorded facts: %v", err)
	}
}

func testOutputSelectedTimeArtifactFailurePreservesCLIError(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	blocked := filepath.Join(home, "blocked-logs")
	if err := os.WriteFile(blocked, []byte("existing destination"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := support.ScaffoldFactory(t, textStreamPromptRunFactoryConfig())
	support.WriteAgentConfig(t, dir, "mock-worker", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
	source := &selectedRunWall{}
	source.nanos.Store(time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC).UnixNano())
	process := support.BuildProcess(t, serviceedges.Edges{Clock: source, ProviderCommandRunner: support.NewStaticSuccessCommandRunner("unexpected success")})
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", dir, "--runtime-log-dir", blocked, "--no-record", "artifact failure"})
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	err := process.Execute(inputs.Input)
	var coded clidiag.CodedError
	if !errors.As(err, &coded) || coded.CLIErrorCode() == "" || strings.Contains(inputs.Stdout(), "unexpected success") {
		t.Fatalf("artifact failure = %v stdout=%q", err, inputs.Stdout())
	}
	preserved, readErr := os.ReadFile(blocked)
	if readErr != nil || string(preserved) != "existing destination" {
		t.Fatalf("artifact destination changed: %q, %v", preserved, readErr)
	}
	t.Logf("artifact error code=%s message=%s", coded.CLIErrorCode(), coded.CLIErrorMessage())
}

func assertInjectedCompletedDiagnostic(t *testing.T, fields map[string]any, requestID, traceID string) {
	t.Helper()
	if fields["request_id"] != requestID || fields["trace_id"] != traceID || fields["status"] != "COMPLETED" || fields["resolved_work_id"] == "" {
		t.Fatalf("CLI invocation diagnostic correlation=%#v", fields)
	}
}
