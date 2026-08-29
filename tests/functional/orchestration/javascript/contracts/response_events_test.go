package contracts

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexChildSessionID             = "codex-js-child-session"
	phaseCheckpointWorkflowFileName = "phase-checkpoint.workflow.js"
	phaseCheckpointWorkflowSource   = `phase("plan");
workflow.checkpoint({ label: "plan-ready", state: { ready: true } });
phase("execute");
return "hello";`
	childProgressWorkflowFileName = "child-progress.workflow.js"
	childProgressWorkflowSource   = `return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    modelProvider: "codex",
    model: "codex-test-model",
  });
  return { label: "child-progress", child: child };
})();`
)

// TestJavaScriptChildProgressPublishesCanonicalResponseEvents proves JavaScript
// child dispatches publish message and tool progress as canonical
// FactoryResponseEvent records on the public Factory Session response-event
// surface after a root-built process run.
func runJavaScriptChildProgressPublishesCanonicalResponseEvents(
	t *testing.T,
	fixture *contractFixture,
) {
	dir := scaffoldChildProgressWorkflow(t)
	started := startChildProgressWorkflow(t, fixture, dir, fixture.nextRequestID("child-progress"))
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.providerCallCount(); got != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 live child invocation", got)
	}
	responseEvents := fixture.responseEvents(t, started.SessionId)
	assertJavaScriptChildProgressResponseEvents(t, responseEvents)
}

// TestJavaScriptTerminalResultFollowsFinalResponseEvent proves the terminal
// Factory Session invocation result becomes observable only after the final
// published FactoryResponseEvent for a JavaScript child dispatch.
func runJavaScriptTerminalResultFollowsFinalResponseEvent(
	t *testing.T,
	fixture *contractFixture,
) {
	dir := scaffoldChildProgressWorkflow(t)
	started := startChildProgressWorkflowAsync(t, fixture, dir, fixture.nextRequestID("terminal-result"))
	if started.SessionId == "" {
		t.Fatal("async session id is empty")
	}

	responseEvents, terminalResult, observations := fixture.observeChildProgressExecutionOrdering(
		t,
		started.SessionId,
	)
	if got := fixture.providerCallCount(); got != 2 {
		t.Fatalf("provider command runner calls = %d, want 2 live child invocations across response-event scenarios", got)
	}
	assertJavaScriptChildProgressResponseEvents(t, responseEvents)
	assertTerminalResultFollowsFinalResponseEvent(t, observations, terminalResult)
}

// TestJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents proves a
// JavaScript Factory publishes ordered orchestrator phase and checkpoint
// Factory Events on the public Factory Session event surface after a root-built
// durable sync execution.
func runJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents(
	t *testing.T,
	fixture *contractFixture,
) {
	dir := scaffoldPhaseCheckpointWorkflow(t)
	providerCalls := fixture.providerCallCount()
	started := startPhaseCheckpointWorkflow(t, fixture, dir, fixture.nextRequestID("phase-checkpoint"))
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.providerCallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d for fake child execution", got, providerCalls)
	}

	events := fixture.factoryEvents(t, started.SessionId)
	assertJavaScriptPhaseCheckpointLifecycleEvents(t, events)
}

func scaffoldChildProgressWorkflow(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-child-progress"})
	if err := os.WriteFile(
		filepath.Join(dir, childProgressWorkflowFileName),
		[]byte(childProgressWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write child progress workflow: %v", err)
	}
	return dir
}

func scaffoldPhaseCheckpointWorkflow(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-phase-checkpoint"})
	if err := os.WriteFile(
		filepath.Join(dir, phaseCheckpointWorkflowFileName),
		[]byte(phaseCheckpointWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write phase checkpoint workflow: %v", err)
	}
	return dir
}

func phaseCheckpointWorkflowRequest(
	dir, requestID string,
) (factoryapi.FactorySessionExecutionRequest, error) {
	workflowPath := filepath.Join(dir, phaseCheckpointWorkflowFileName)
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	}, nil
}

func startPhaseCheckpointWorkflow(
	t *testing.T,
	fixture *contractFixture,
	dir, requestID string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	requestPayload, err := phaseCheckpointWorkflowRequest(dir, requestID)
	if err != nil {
		t.Fatalf("build phase checkpoint workflow request: %v", err)
	}
	return fixture.startSync(t, requestPayload, dir)
}

func childProgressWorkflowRequest(
	dir, requestID string,
) (factoryapi.FactorySessionExecutionRequest, error) {
	workflowPath := filepath.Join(dir, childProgressWorkflowFileName)
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	}, nil
}

func startChildProgressWorkflow(
	t *testing.T,
	fixture *contractFixture,
	dir, requestID string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	requestPayload, err := childProgressWorkflowRequest(dir, requestID)
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	return fixture.startSync(t, requestPayload, dir)
}

func startChildProgressWorkflowAsync(
	t *testing.T,
	fixture *contractFixture,
	dir, requestID string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	requestPayload, err := childProgressWorkflowRequest(dir, requestID)
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	return fixture.startAsync(t, requestPayload, dir)
}

type executionObservationKind string

const (
	executionObservationResponseEvent  executionObservationKind = "response_event"
	executionObservationTerminalResult executionObservationKind = "terminal_result"
)

type executionObservation struct {
	order int64
	kind  executionObservationKind
}

func observeChildProgressExecutionOrdering(
	t *testing.T,
	serverURL, sessionID string,
) ([]factoryapi.FactoryResponseEvent, factoryapi.FactorySessionResult, []executionObservation) {
	t.Helper()

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(serverURL, sessionID),
	)
	var (
		events       []factoryapi.FactoryResponseEvent
		observations []executionObservation
	)
	for {
		result := stream.TryNextFrameResult(contractFixtureTimeout)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
			observations = append(observations, executionObservation{
				order: int64(len(observations) + 1),
				kind:  executionObservationResponseEvent,
			})
		case support.FactoryResponseEventStreamOutcomeEOF:
			stream.WaitClosed(contractFixtureTimeout)
			terminal := support.GetJSON[factoryapi.FactorySessionResult](
				t,
				strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/results?mode=final",
			)
			observations = append(observations, executionObservation{
				order: int64(len(observations) + 1),
				kind:  executionObservationTerminalResult,
			})
			return events, terminal, observations
		case support.FactoryResponseEventStreamOutcomeTimeout:
			t.Fatalf("timed out reading child response-event stream: %s", result.Diagnostic())
		case support.FactoryResponseEventStreamOutcomeReadError,
			support.FactoryResponseEventStreamOutcomeCanceled:
			t.Fatalf("child response-event stream ended before terminal event: %s", result.Diagnostic())
		default:
			t.Fatalf("unexpected child response-event stream outcome: %s", result.Diagnostic())
		}
	}
}

// readFactoryResponseEventsUntilClosed uses the stream's terminal EOF as the
// completion signal. The bounded per-frame wait is only a fail-fast guard for
// a broken public stream; it is not a quiet-period or scheduler poll.
func readFactoryResponseEventsUntilClosed(
	t *testing.T,
	serverURL, sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(serverURL, sessionID),
	)
	var events []factoryapi.FactoryResponseEvent
	for {
		result := stream.TryNextFrameResult(contractFixtureTimeout)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
		case support.FactoryResponseEventStreamOutcomeEOF:
			stream.WaitClosed(contractFixtureTimeout)
			return events
		case support.FactoryResponseEventStreamOutcomeTimeout,
			support.FactoryResponseEventStreamOutcomeReadError,
			support.FactoryResponseEventStreamOutcomeCanceled:
			t.Fatalf("response-event stream ended before EOF: %s", result.Diagnostic())
		default:
			t.Fatalf("unexpected response-event stream outcome: %s", result.Diagnostic())
		}
	}
}

func assertTerminalResultFollowsFinalResponseEvent(
	t *testing.T,
	observations []executionObservation,
	terminal factoryapi.FactorySessionResult,
) {
	t.Helper()
	if terminal.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("terminal result status = %q, want FINAL", terminal.ResultStatus)
	}
	if terminal.SessionStatus == nil ||
		*terminal.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("terminal session status = %#v, want SUCCEEDED", terminal.SessionStatus)
	}
	if terminal.PrimaryResult == nil || len(*terminal.PrimaryResult) == 0 {
		t.Fatalf("terminal result = %#v, want primary invocation output", terminal)
	}

	terminalIndex := -1
	for index, observation := range observations {
		switch observation.kind {
		case executionObservationTerminalResult:
			if terminalIndex >= 0 {
				t.Fatalf("terminal observations = %d, want exactly 1", len(observations))
			}
			terminalIndex = index
		case executionObservationResponseEvent:
		default:
			t.Fatalf("unexpected observation kind = %q", observation.kind)
		}
	}
	if terminalIndex < 0 {
		t.Fatal("terminal invocation result was not observed")
	}
	for index := terminalIndex + 1; index < len(observations); index++ {
		if observations[index].kind == executionObservationResponseEvent {
			t.Fatalf(
				"response event observation[%d] followed terminal result observation[%d]",
				index,
				terminalIndex,
			)
		}
	}
	sawResponseBeforeTerminal := false
	for index := 0; index < terminalIndex; index++ {
		if observations[index].kind == executionObservationResponseEvent {
			sawResponseBeforeTerminal = true
			break
		}
	}
	if !sawResponseBeforeTerminal {
		t.Fatal("terminal invocation result appeared before any response event")
	}
}

func assertJavaScriptPhaseCheckpointLifecycleEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	var lifecycle []factoryapi.FactoryEvent
	previousSessionSequence := -1
	for index, event := range events {
		if event.Context.SessionSequence == nil {
			continue
		}
		if *event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf(
				"Factory Session sequence %d follows %d at event %d",
				*event.Context.SessionSequence,
				previousSessionSequence,
				index,
			)
		}
		previousSessionSequence = *event.Context.SessionSequence
		switch event.Type {
		case factoryapi.FactoryEventTypeOrchestratorPhaseChanged,
			factoryapi.FactoryEventTypeOrchestratorCheckpointWritten:
			lifecycle = append(lifecycle, event)
		}
	}

	want := []struct {
		eventType factoryapi.FactoryEventType
		phaseName string
		status    factoryapi.OrchestratorPhaseStatus
	}{
		{factoryapi.FactoryEventTypeOrchestratorPhaseChanged, "plan", factoryapi.ACTIVE},
		{factoryapi.FactoryEventTypeOrchestratorCheckpointWritten, "plan", ""},
		{factoryapi.FactoryEventTypeOrchestratorPhaseChanged, "plan", factoryapi.COMPLETED},
		{factoryapi.FactoryEventTypeOrchestratorPhaseChanged, "execute", factoryapi.ACTIVE},
		{factoryapi.FactoryEventTypeOrchestratorPhaseChanged, "execute", factoryapi.COMPLETED},
	}
	if len(lifecycle) != len(want) {
		t.Fatalf("JavaScript lifecycle event count = %d, want %d: %#v", len(lifecycle), len(want), lifecycle)
	}
	for index, expected := range want {
		event := lifecycle[index]
		if event.Context.PhaseName == nil || *event.Context.PhaseName != expected.phaseName {
			t.Fatalf("JavaScript lifecycle event %d = %#v, want phase %q", index, event, expected.phaseName)
		}
		if event.Type != expected.eventType {
			t.Fatalf("JavaScript lifecycle event %d type = %q, want %q", index, event.Type, expected.eventType)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeOrchestratorPhaseChanged:
			payload, err := event.Payload.AsOrchestratorPhaseChangedEventPayload()
			if err != nil {
				t.Fatalf("decode phase event %d payload: %v", index, err)
			}
			if payload.PhaseStatus != expected.status {
				t.Fatalf("phase event %d status = %q, want %q", index, payload.PhaseStatus, expected.status)
			}
		case factoryapi.FactoryEventTypeOrchestratorCheckpointWritten:
			payload, err := event.Payload.AsOrchestratorCheckpointWrittenEventPayload()
			if err != nil {
				t.Fatalf("decode checkpoint event payload: %v", err)
			}
			if payload.Label != "plan-ready" || payload.ResumabilityStatus == "" || event.Context.CheckpointId == nil {
				t.Fatalf(
					"checkpoint event = %#v payload = %#v, want public plan-ready resumable checkpoint",
					event,
					payload,
				)
			}
		}
	}
}

func assertJavaScriptChildProgressResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("response events are empty, want child message/tool progress")
	}

	var dispatchID string
	previousSequence := int64(0)
	sawMessage := false
	sawTool := false
	for index, event := range events {
		if event.Sequence <= previousSequence {
			t.Fatalf("response event[%d] sequence = %d follows %d", index, event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		if event.DispatchId == nil || strings.TrimSpace(*event.DispatchId) == "" {
			t.Fatalf("response event[%d] = %#v, want dispatch correlation", index, event)
		}
		if dispatchID == "" {
			dispatchID = *event.DispatchId
		}
		if *event.DispatchId != dispatchID {
			t.Fatalf("response event[%d] dispatch = %q, want %q", index, *event.DispatchId, dispatchID)
		}
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			sawMessage = true
		case factoryapi.FactoryResponseEventKindTool:
			sawTool = true
		}
	}
	if !sawMessage {
		t.Fatalf("response events = %#v, want at least one MESSAGE progress event", events)
	}
	if !sawTool {
		t.Fatalf("response events = %#v, want at least one TOOL progress event", events)
	}
}

func getFactoryEventsForSessionAt(
	t *testing.T,
	serverURL, sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(serverURL, "/") +
		"/factory-sessions/" + sessionID + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory session events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory session events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET factory session events status = %d: %s", response.StatusCode, body)
	}

	var collected []factoryapi.FactoryEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode factory session event: %v", err)
		}
		collected = append(collected, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read factory session events: %v", err)
	}
	return collected
}

func codexChildProgressStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"thread.started","thread_id":` + mustJSONString(sessionID) + `}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"message-working","type":"agent_message","text":"working"}}`,
		`{"type":"item.started","item":{"id":"call-1","type":"command_execution","command":"read README.md"}}`,
		`{"type":"item.completed","item":{"id":"call-1","type":"command_execution","command":"read README.md","aggregated_output":"success"}}`,
		`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":` + mustJSONString(result) + `}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}
	return []byte(strings.Join(records, "\n"))
}

func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
