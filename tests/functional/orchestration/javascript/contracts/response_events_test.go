package contracts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	cursorChildSessionID            = "cursor-js-child-session"
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
    modelProvider: "cursor",
    model: "cursor-test-model",
  });
  return { label: "child-progress", child: child };
})();`
)

// TestJavaScriptChildProgressPublishesCanonicalResponseEvents proves JavaScript
// child dispatches publish message and tool progress as canonical
// FactoryResponseEvent records on the public Factory Session response-event
// surface after a root-built process run.
func TestJavaScriptChildProgressPublishesCanonicalResponseEvents(t *testing.T) {
	dir := scaffoldChildProgressWorkflow(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorChildProgressStream(cursorChildSessionID, "Child summary COMPLETE"),
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})

	started := startChildProgressWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 live child invocation", runner.CallCount())
	}
	responseEvents := support.GetFactoryResponseEventsAt(t, server.URL(), started.SessionId)
	assertJavaScriptChildProgressResponseEvents(t, responseEvents)
}

// TestJavaScriptTerminalResultFollowsFinalResponseEvent proves the terminal
// Factory Session invocation result becomes observable only after the final
// published FactoryResponseEvent for a JavaScript child dispatch.
func TestJavaScriptTerminalResultFollowsFinalResponseEvent(t *testing.T) {
	dir := scaffoldChildProgressWorkflow(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorChildProgressStream(cursorChildSessionID, "Child summary COMPLETE"),
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})

	started := startChildProgressWorkflowAsync(t, server.URL(), dir)
	if started.SessionId == "" {
		t.Fatal("async session id is empty")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	responseEvents, terminalResult, observations := observeChildProgressExecutionOrdering(
		ctx,
		t,
		server.URL(),
		started.SessionId,
	)
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 live child invocation", runner.CallCount())
	}
	assertJavaScriptChildProgressResponseEvents(t, responseEvents)
	assertTerminalResultFollowsFinalResponseEvent(t, observations, terminalResult)
}

// TestJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents proves a
// mock-worker JavaScript Factory publishes ordered orchestrator phase and
// checkpoint Factory Events on the public Factory Session event surface after
// a root-built durable sync execution.
func TestJavaScriptPhaseCheckpointLifecyclePublishesCanonicalFactoryEvents(t *testing.T) {
	dir := scaffoldPhaseCheckpointWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})

	started := startPhaseCheckpointWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	events := getFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
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

func phaseCheckpointWorkflowRequest(dir string) (factoryapi.FactorySessionExecutionRequest, error) {
	workflowPath := filepath.Join(dir, phaseCheckpointWorkflowFileName)
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-phase-checkpoint-response-events",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	}, nil
}

func startPhaseCheckpointWorkflow(t *testing.T, serverURL, dir string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	requestPayload, err := phaseCheckpointWorkflowRequest(dir)
	if err != nil {
		t.Fatalf("build phase checkpoint workflow request: %v", err)
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal phase checkpoint workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build phase checkpoint workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start phase checkpoint workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start phase checkpoint workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode phase checkpoint workflow response: %v", err)
	}
	return started
}

func childProgressWorkflowRequest(dir string) (factoryapi.FactorySessionExecutionRequest, error) {
	workflowPath := filepath.Join(dir, childProgressWorkflowFileName)
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-child-progress-response-events",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	}, nil
}

func startChildProgressWorkflow(t *testing.T, serverURL, dir string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	requestPayload, err := childProgressWorkflowRequest(dir)
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal child progress workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start child progress workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start child progress workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode child progress workflow response: %v", err)
	}
	return started
}

func startChildProgressWorkflowAsync(t *testing.T, serverURL, dir string) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	requestPayload, err := childProgressWorkflowRequest(dir)
	if err != nil {
		t.Fatalf("build child progress workflow request: %v", err)
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal child progress workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build async child progress workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start async child progress workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start async child progress workflow status = %d: %s", response.StatusCode, body)
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode async child progress workflow response: %v", err)
	}
	return started
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
	ctx context.Context,
	t *testing.T,
	serverURL, sessionID string,
) ([]factoryapi.FactoryResponseEvent, factoryapi.FactorySessionResult, []executionObservation) {
	t.Helper()

	var (
		mu           sync.Mutex
		order        atomic.Int64
		observations []executionObservation
		events       []factoryapi.FactoryResponseEvent
		terminal     factoryapi.FactorySessionResult
	)
	record := func(kind executionObservationKind) {
		mu.Lock()
		observations = append(observations, executionObservation{
			order: order.Add(1),
			kind:  kind,
		})
		mu.Unlock()
	}

	sseReady := make(chan struct{})
	sseDone := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		defer close(sseDone)
		err := streamFactoryResponseEvents(ctx, serverURL, sessionID, func(event factoryapi.FactoryResponseEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
			record(executionObservationResponseEvent)
		}, func() {
			close(sseReady)
		})
		errCh <- err
	}()
	go func() {
		select {
		case <-sseReady:
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		}

		result, err := pollForFinalSessionResult(ctx, serverURL, sessionID)
		if err != nil {
			errCh <- err
			return
		}
		terminal = *result

		// Record terminal observation only after the response-event stream
		// finishes delivering retained events so concurrent polling cannot win
		// the race before SSE catches up on fast CI hosts.
		select {
		case <-sseDone:
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		}
		record(executionObservationTerminalResult)
		errCh <- nil
	}()

	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("observe child progress execution ordering: %v", err)
		}
	}
	return events, terminal, observations
}

func streamFactoryResponseEvents(
	ctx context.Context,
	serverURL, sessionID string,
	onEvent func(factoryapi.FactoryResponseEvent),
	onConnected func(),
) error {
	endpoint := strings.TrimSuffix(serverURL, "/") +
		"/factory-sessions/" + sessionID + "/response-events"
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("build factory response events request: %w", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return fmt.Errorf("GET factory response events: %w", err)
		}
		if response.StatusCode == http.StatusNotFound {
			response.Body.Close()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-retry.C:
				continue
			}
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return fmt.Errorf("GET factory response events status = %d: %s", response.StatusCode, body)
		}
		if onConnected != nil {
			onConnected()
			onConnected = nil
		}

		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event factoryapi.FactoryResponseEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				response.Body.Close()
				return fmt.Errorf("decode factory response event: %w", err)
			}
			onEvent(event)
		}
		err = scanner.Err()
		response.Body.Close()
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("read factory response events: %w", err)
		}
		return nil
	}
}

func pollForFinalSessionResult(
	ctx context.Context,
	serverURL, sessionID string,
) (*factoryapi.FactorySessionResult, error) {
	endpoint := strings.TrimSuffix(serverURL, "/") +
		"/factory-sessions/" + sessionID + "/results?mode=final"
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build factory session result request: %w", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("GET factory session result: %w", err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read factory session result: %w", readErr)
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET factory session result status = %d: %s", response.StatusCode, body)
		}
		var result factoryapi.FactorySessionResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode factory session result: %w", err)
		}
		if result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
			return &result, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
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

func cursorChildProgressStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"system","subtype":"init","session_id":"` + sessionID + `"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"working"}]},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-1","tool_call":{"readToolCall":{"result":{"success":{}}}},"session_id":"` + sessionID + `"}`,
		string(cursorChildTerminalRecord(sessionID, result)),
	}
	return []byte(strings.Join(records, "\n"))
}

func cursorChildTerminalRecord(sessionID, result string) []byte {
	return []byte(
		`{"type":"result","subtype":"success","is_error":false,"result":` +
			mustJSONString(result) + `,"session_id":` + mustJSONString(sessionID) + `}`,
	)
}

func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
