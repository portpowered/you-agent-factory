package functional_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const scriptEventsSecretEnv = "SCRIPT_EVENTS_API_TOKEN"
const scriptEventsSecretValue = "raw-script-events-secret-value"

type processErrorCommandRunner struct {
	stderr string
}

func (r processErrorCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stderr: []byte(r.stderr)}, errors.New("exec: file not found")
}

func TestScriptEvents_ScriptWorkersEmitRequestBoundaryEventInCanonicalHistory(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	writeScriptWorkerArgsFixture(t, dir)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-script-request-event",
		WorkTypeID: "task",
		TraceID:    "trace-script-request-event",
		Payload:    []byte("script input"),
		Tags:       map[string]string{"priority": "high"},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(successRunner("script-output-ok")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	events := runHarnessAndLoadEvents(t, h)
	indices := requireScriptRequestEventIndices(t, events)

	assertFunctionalScriptRequestBoundaryEvent(t, events, indices, "work-script-request-event")
}

func TestScriptEvents_ScriptWorkersEmitResponseBoundaryEventInCanonicalHistory(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	recordPath := filepath.Join(t.TempDir(), "script-events-success.replay.json")
	t.Setenv(scriptEventsSecretEnv, scriptEventsSecretValue)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-script-response-event",
		WorkTypeID: "task",
		TraceID:    "trace-script-response-event",
		Payload:    []byte("script input"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(successRunner("script-output-ok")),
		testutil.WithRecordPath(recordPath),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	events := runHarnessAndLoadEvents(t, h)
	indices := requireScriptResponseEventIndices(t, events)

	assertFunctionalScriptResponseBoundaryEvent(t, events, indices, expectedFunctionalScriptResponse{
		workID:     "work-script-response-event",
		outcome:    factoryapi.ScriptExecutionOutcomeSucceeded,
		exitCode:   intPtrFunctionalTest(0),
		stdout:     "script-output-ok",
		stderr:     "",
		forbidden:  []string{`"stdin"`, `"env"`, `"SCRIPT_API_TOKEN"`, scriptEventsSecretValue},
		trimStdout: true,
	})

	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertScriptEventsRecordedInArtifact(t, events, artifact.Events)
	assertReplayArtifactDoesNotContainRawValue(t, recordPath, scriptEventsSecretValue)
}

func TestScriptEvents_ScriptWorkersEmitProcessFailureBoundaryEventInCanonicalHistoryAndArtifact(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	recordPath := filepath.Join(t.TempDir(), "script-events-process-error.replay.json")
	t.Setenv(scriptEventsSecretEnv, scriptEventsSecretValue)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-script-process-error-event",
		WorkTypeID: "task",
		TraceID:    "trace-script-process-error-event",
		Payload:    []byte("script input"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(processErrorCommandRunner{stderr: "launch failed"}),
		testutil.WithRecordPath(recordPath),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	events := runHarnessAndLoadEvents(t, h)
	indices := requireScriptResponseEventIndices(t, events)

	processError := factoryapi.ScriptFailureTypeProcessError
	assertFunctionalScriptResponseBoundaryEvent(t, events, indices, expectedFunctionalScriptResponse{
		workID:    "work-script-process-error-event",
		outcome:   factoryapi.ScriptExecutionOutcomeProcessError,
		failure:   &processError,
		stdout:    "",
		stderr:    "launch failed",
		forbidden: []string{`"stdin"`, `"env"`, `"SCRIPT_API_TOKEN"`, scriptEventsSecretValue},
	})

	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertScriptEventsRecordedInArtifact(t, events, artifact.Events)
	assertReplayArtifactDoesNotContainRawValue(t, recordPath, scriptEventsSecretValue)
}

type scriptBoundaryEventIndices struct {
	dispatch  int
	request   int
	response  int
	completed int
}

type expectedFunctionalScriptResponse struct {
	workID     string
	outcome    factoryapi.ScriptExecutionOutcome
	failure    *factoryapi.ScriptFailureType
	exitCode   *int
	stdout     string
	stderr     string
	forbidden  []string
	trimStdout bool
}

func writeScriptWorkerArgsFixture(t *testing.T, dir string) {
	t.Helper()

	agentsMD := strings.Join([]string{
		"---",
		"type: SCRIPT_WORKER",
		"command: script-tool",
		"args:",
		`  - "--work"`,
		`  - "{{ (index .Inputs 0).WorkID }}"`,
		`  - "--priority"`,
		`  - '{{ index (index .Inputs 0).Tags "priority" }}'`,
		"---",
		"",
	}, "\n")
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func runHarnessAndLoadEvents(t *testing.T, h *testutil.ServiceTestHarness) []factoryapi.FactoryEvent {
	t.Helper()

	h.RunUntilComplete(t, 5*time.Second)

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	return events
}

func requireScriptRequestEventIndices(t *testing.T, events []factoryapi.FactoryEvent) scriptBoundaryEventIndices {
	t.Helper()

	indices := scriptBoundaryEventIndices{
		dispatch:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0),
		request:   indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeScriptRequest, 0),
		completed: indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, 0),
	}
	if indices.dispatch < 0 || indices.request < 0 || indices.completed < 0 {
		t.Fatalf("event order = %v, want dispatch-request, script-request, dispatch-response", functionalEventTypes(events))
	}
	return indices
}

func requireScriptResponseEventIndices(t *testing.T, events []factoryapi.FactoryEvent) scriptBoundaryEventIndices {
	t.Helper()

	indices := scriptBoundaryEventIndices{
		dispatch:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0),
		request:   indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeScriptRequest, 0),
		response:  indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeScriptResponse, 0),
		completed: indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, 0),
	}
	if indices.dispatch < 0 || indices.request < 0 || indices.response < 0 || indices.completed < 0 {
		t.Fatalf("event order = %v, want dispatch-request, script-request, script-response, dispatch-response", functionalEventTypes(events))
	}
	return indices
}

func assertFunctionalScriptRequestBoundaryEvent(t *testing.T, events []factoryapi.FactoryEvent, indices scriptBoundaryEventIndices, workID string) {
	t.Helper()

	dispatch, err := events[indices.dispatch].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode dispatch request payload: %v", err)
	}
	request, err := events[indices.request].Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("decode script request payload: %v", err)
	}

	dispatchID := stringValueForFunctionalTest(events[indices.dispatch].Context.DispatchId)
	completedDispatchID := stringValueForFunctionalTest(events[indices.completed].Context.DispatchId)
	if request.DispatchId != dispatchID || completedDispatchID != dispatchID {
		t.Fatalf("dispatch correlation mismatch: dispatch=%s request=%s completed=%s", dispatchID, request.DispatchId, completedDispatchID)
	}
	if request.TransitionId != dispatch.TransitionId {
		t.Fatalf("transition correlation mismatch: dispatch=%s request=%s", dispatch.TransitionId, request.TransitionId)
	}
	if request.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", request.Attempt)
	}
	if request.ScriptRequestId != dispatchID+"/script-request/1" {
		t.Fatalf("script request id = %q, want dispatch-derived stable ID", request.ScriptRequestId)
	}
	if request.Command != "script-tool" {
		t.Fatalf("command = %q, want script-tool", request.Command)
	}
	if strings.Join(request.Args, " ") != "--work "+workID+" --priority high" {
		t.Fatalf("args = %#v, want resolved work-id and tag args", request.Args)
	}
	if stringValueForFunctionalTest(events[indices.request].Context.DispatchId) != dispatchID {
		t.Fatalf("event context dispatchId = %q, want %q", stringValueForFunctionalTest(events[indices.request].Context.DispatchId), dispatchID)
	}
	if got := events[indices.request].Context.WorkIds; got == nil || len(*got) != 1 || (*got)[0] != workID {
		t.Fatalf("event context workIds = %#v, want seeded work ID", got)
	}

	assertFunctionalScriptEventDoesNotLeak(t, events[indices.request], []string{`"stdin"`, `"env"`, `"SCRIPT_API_TOKEN"`})
}

func assertFunctionalScriptResponseBoundaryEvent(t *testing.T, events []factoryapi.FactoryEvent, indices scriptBoundaryEventIndices, want expectedFunctionalScriptResponse) {
	t.Helper()

	request, err := events[indices.request].Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("decode script request payload: %v", err)
	}
	response, err := events[indices.response].Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("decode script response payload: %v", err)
	}

	completedDispatchID := stringValueForFunctionalTest(events[indices.completed].Context.DispatchId)
	if response.ScriptRequestId != request.ScriptRequestId {
		t.Fatalf("script request correlation mismatch: request=%s response=%s", request.ScriptRequestId, response.ScriptRequestId)
	}
	if response.DispatchId != request.DispatchId || completedDispatchID != request.DispatchId {
		t.Fatalf("dispatch correlation mismatch: request=%s response=%s completed=%s", request.DispatchId, response.DispatchId, completedDispatchID)
	}
	if response.TransitionId != request.TransitionId {
		t.Fatalf("transition correlation mismatch: request=%s response=%s", request.TransitionId, response.TransitionId)
	}
	if response.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", response.Attempt)
	}
	if response.Outcome != want.outcome {
		t.Fatalf("response outcome = %s, want %s", response.Outcome, want.outcome)
	}
	if !equalOptionalIntFunctionalTest(response.ExitCode, want.exitCode) {
		t.Fatalf("response exit code = %#v, want %#v", response.ExitCode, want.exitCode)
	}
	if !equalOptionalScriptFailureTypeFunctionalTest(response.FailureType, want.failure) {
		t.Fatalf("response failure type = %#v, want %#v", response.FailureType, want.failure)
	}
	if actualStdout := normalizeFunctionalStdout(response.Stdout, want.trimStdout); actualStdout != want.stdout {
		t.Fatalf("response stdout = %q, want %q", actualStdout, want.stdout)
	}
	if response.Stderr != want.stderr {
		t.Fatalf("response stderr = %q, want %q", response.Stderr, want.stderr)
	}
	if response.DurationMillis < 0 {
		t.Fatalf("response duration millis = %d, want non-negative", response.DurationMillis)
	}
	if stringValueForFunctionalTest(events[indices.response].Context.DispatchId) != request.DispatchId {
		t.Fatalf("response context dispatchId = %q, want %q", stringValueForFunctionalTest(events[indices.response].Context.DispatchId), request.DispatchId)
	}
	if got := events[indices.response].Context.WorkIds; got == nil || len(*got) != 1 || (*got)[0] != want.workID {
		t.Fatalf("response context workIds = %#v, want seeded work ID", got)
	}

	assertFunctionalScriptEventDoesNotLeak(t, events[indices.response], want.forbidden)
}

func assertScriptEventsRecordedInArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, recordedEvents []factoryapi.FactoryEvent) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(recordedEvents))
	for _, event := range recordedEvents {
		recordedByID[event.Id] = event
	}

	for _, live := range liveEvents {
		if live.Type != factoryapi.FactoryEventTypeScriptRequest && live.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}

		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("recorded artifact missing script event %s from live history; artifact events=%v", live.Id, functionalEventTypes(recordedEvents))
		}
		if recorded.Type != live.Type {
			t.Fatalf("recorded script event %s = type %s, live type %s", live.Id, recorded.Type, live.Type)
		}

		liveJSON, err := json.Marshal(live)
		if err != nil {
			t.Fatalf("marshal live script event %s: %v", live.Id, err)
		}
		recordedJSON, err := json.Marshal(recorded)
		if err != nil {
			t.Fatalf("marshal recorded script event %s: %v", recorded.Id, err)
		}
		if string(recordedJSON) != string(liveJSON) {
			t.Fatalf("recorded script event %s does not match live history\nrecorded=%s\nlive=%s", live.Id, recordedJSON, liveJSON)
		}
	}
}

func assertFunctionalScriptEventDoesNotLeak(t *testing.T, event factoryapi.FactoryEvent, forbidden []string) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal script event: %v", err)
	}
	body := string(encoded)
	for _, value := range forbidden {
		if strings.Contains(body, value) {
			t.Fatalf("script event leaked %s: %s", value, body)
		}
	}
}

func normalizeFunctionalStdout(stdout string, trim bool) string {
	if trim {
		return strings.TrimSpace(stdout)
	}
	return stdout
}

func equalOptionalIntFunctionalTest(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalScriptFailureTypeFunctionalTest(left, right *factoryapi.ScriptFailureType) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func intPtrFunctionalTest(value int) *int {
	return &value
}
