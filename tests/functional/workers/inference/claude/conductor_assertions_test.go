package claude

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertClaudeWork(t *testing.T, scenario claudeScenario, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, scenario.wantWorkState); got != 1 {
		t.Fatalf("%s terminal Work state count = %d, want 1; listed=%#v", scenario.name, got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); scenario.wantWorkState == "task:failed" && got != 0 {
		t.Fatalf("%s completed Work count = %d, want 0", scenario.name, got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); scenario.wantWorkState == "task:done" && got != 0 {
		t.Fatalf("%s failed Work count = %d, want 0", scenario.name, got)
	}

	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != scenario.workID {
			continue
		}
		found++
		if support.StringPointerValue(item.RequestId) != scenario.requestID {
			t.Fatalf("%s Work request id = %q, want %q", scenario.name, support.StringPointerValue(item.RequestId), scenario.requestID)
		}
		if scenario.wantFailure != "" {
			if item.FailureDetail == nil || !strings.Contains(item.FailureDetail.Message, scenario.wantFailure) {
				t.Fatalf("%s Work failure detail = %#v, want %q", scenario.name, item.FailureDetail, scenario.wantFailure)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q", scenario.name, found, scenario.workID)
	}
}

func assertClaudeDispatch(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) []string {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != scenario.wantDispatches {
		t.Fatalf("%s dispatch observations = %#v, want %d", scenario.name, dispatches, scenario.wantDispatches)
	}
	dispatchIDs := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.DispatchID == "" {
			t.Fatalf("%s dispatch identity is empty", scenario.name)
		}
		if !support.DispatchObservationIncludesWork(dispatch, scenario.workID) {
			t.Fatalf("%s dispatch %q omitted Work %q: %#v", scenario.name, dispatch.DispatchID, scenario.workID, dispatch)
		}
		if dispatch.Response == nil {
			t.Fatalf("%s dispatch %q has no response", scenario.name, dispatch.DispatchID)
		}
		if dispatch.Response.Outcome != scenario.wantOutcome {
			t.Fatalf("%s dispatch outcome = %q, want %q", scenario.name, dispatch.Response.Outcome, scenario.wantOutcome)
		}
		if scenario.wantWorkState == "task:failed" {
			if scenario.wantFailure == "" {
				t.Fatalf("%s failed dispatch has no expected failure message", scenario.name)
			}
			if dispatch.Response.FailureDetail == nil || !strings.Contains(dispatch.Response.FailureDetail.Message, scenario.wantFailure) {
				t.Fatalf("%s dispatch failure detail = %#v, want %q", scenario.name, dispatch.Response.FailureDetail, scenario.wantFailure)
			}
		}
		dispatchIDs = append(dispatchIDs, dispatch.DispatchID)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s Factory Event %q session id = %q, want %q", scenario.name, event.Id, *event.Context.SessionId, sessionID)
		}
	}
	return dispatchIDs
}

func assertClaudeCommand(t *testing.T, router *claudeCommandRouter, scenario claudeScenario) {
	t.Helper()
	requests := scenario.runner.Requests()
	if len(requests) != scenario.wantProviderCalls {
		t.Fatalf("%s routed provider calls = %d, want %d; requests=%#v", scenario.name, len(requests), scenario.wantProviderCalls, requests)
	}
	routed := router.callsFor(scenario.factoryDir)
	if len(routed) != scenario.wantProviderCalls {
		t.Fatalf("%s immutable route calls = %d, want %d; calls=%#v", scenario.name, len(routed), scenario.wantProviderCalls, routed)
	}
	for index, routedCall := range routed {
		request := routedCall.request
		if request.WorkDir != requests[index].WorkDir {
			t.Fatalf("%s router WorkDir = %q, runner WorkDir = %q", scenario.name, request.WorkDir, requests[index].WorkDir)
		}
		if request.Command != claudeConductorProcessCommand {
			t.Fatalf("%s command = %q, want claude", scenario.name, request.Command)
		}
		if request.WorkDir != scenario.factoryDir {
			t.Fatalf("%s command WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
		}
		if !containsArgPair(request.Args, "--model", scenario.model) {
			t.Fatalf("%s args = %#v, want --model %s", scenario.name, request.Args, scenario.model)
		}
		if !containsArgPair(request.Args, "--output-format", "stream-json") {
			t.Fatalf("%s args = %#v, want Claude stream-json invocation", scenario.name, request.Args)
		}
	}
}

func assertClaudeProviderSession(
	t *testing.T,
	scenario claudeScenario,
	events []factoryapi.FactoryEvent,
) string {
	t.Helper()
	if scenario.providerSessionID == "" {
		return ""
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse && event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		observation, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("%s decode provider response: %v", scenario.name, err)
		}
		if observation.ProviderSession == nil || observation.ProviderSession.Id == nil {
			continue
		}
		got := strings.TrimSpace(*observation.ProviderSession.Id)
		if got != scenario.providerSessionID {
			t.Fatalf("%s Provider Session id = %q, want %q", scenario.name, got, scenario.providerSessionID)
		}
		return got
	}
	t.Fatalf("%s missing Provider Session identity %q", scenario.name, scenario.providerSessionID)
	return ""
}

func assertClaudeEventScope(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s Factory Event stream is empty", scenario.name)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
		}
		if event.Context.RequestId != nil && *event.Context.RequestId != scenario.requestID {
			t.Fatalf("%s event %q request id = %q, want %q", scenario.name, event.Id, *event.Context.RequestId, scenario.requestID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if workID != scenario.workID {
					t.Fatalf("%s event %q Work id = %q, want %q", scenario.name, event.Id, workID, scenario.workID)
				}
			}
		}
	}
	if scenario.wantWorkState != "task:failed" {
		return
	}
	if scenario.wantFailure == "" {
		t.Fatalf("%s failed Factory Event stream has no expected failure message", scenario.name)
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("%s marshal Factory Events: %v", scenario.name, err)
	}
	text := string(payload)
	if !strings.Contains(text, scenario.wantFailure) {
		t.Fatalf("%s Factory Events missing expected failure %q: %s", scenario.name, scenario.wantFailure, text)
	}
	if strings.Contains(text, "Claude command did not complete successfully") {
		t.Fatalf("%s Factory Events used Claude-local cancellation fallback: %s", scenario.name, text)
	}
}

func assertClaudeResponseEvents(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	responseEvents []factoryapi.FactoryResponseEvent,
) []string {
	t.Helper()
	if len(responseEvents) == 0 {
		t.Fatalf("%s response-event stream is empty", scenario.name)
	}
	ids := make([]string, 0, len(responseEvents))
	seen := make(map[string]struct{}, len(responseEvents))
	var previousSequence int64
	for index, event := range responseEvents {
		if event.FactorySessionId != sessionID {
			t.Fatalf("%s response event[%d] session id = %q, want %q", scenario.name, index, event.FactorySessionId, sessionID)
		}
		if strings.TrimSpace(event.EventId) == "" {
			t.Fatalf("%s response event[%d] has empty event id", scenario.name, index)
		}
		if _, exists := seen[event.EventId]; exists {
			t.Fatalf("%s response event id %q is duplicated", scenario.name, event.EventId)
		}
		seen[event.EventId] = struct{}{}
		if index > 0 && event.Sequence <= previousSequence {
			t.Fatalf("%s response event sequence[%d] = %d, previous = %d", scenario.name, index, event.Sequence, previousSequence)
		}
		if scenario.providerSessionID != "" && event.ProviderSessionRef != nil && *event.ProviderSessionRef != scenario.providerSessionID {
			t.Fatalf("%s response event[%d] Provider Session ref = %q, want %q", scenario.name, index, *event.ProviderSessionRef, scenario.providerSessionID)
		}
		previousSequence = event.Sequence
		ids = append(ids, event.EventId)
	}
	return ids
}

func readClaudeResponseEventsUntilTerminal(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	timeout time.Duration,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	// Response Events arrive as multiple frames, and no single read can identify
	// the terminal phase while preserving the public event order. Consume the
	// already-open SSE stream until its terminal frame; the shared deadline keeps
	// this deterministic and bounded. Status polling or a sleep cannot replace
	// this observation because they do not prove the response-event sequence.
	deadline := time.Now().Add(timeout)
	events := make([]factoryapi.FactoryResponseEvent, 0, 8)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for terminal Claude response event after %s; got %d events", timeout, len(events))
		}
		result := stream.TryNextFrameResult(remaining)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("Claude response stream ended before terminal event: %s", result.Diagnostic())
		}
		event := result.Frame.Event
		events = append(events, event)
		if isClaudeTerminalResponseEvent(event) {
			return events
		}
	}
}

func isClaudeTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindRun {
		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseCompleted,
			factoryapi.FactoryResponseEventPhaseFailed,
			factoryapi.FactoryResponseEventPhaseCanceled:
			return true
		}
	}
	return event.Kind == factoryapi.FactoryResponseEventKindError &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}

func assertClaudeGoldenScenario(
	t *testing.T,
	scenario claudeScenario,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	if scenario.golden == nil {
		return
	}

	switch scenario.golden.Manifest.ID {
	case "claude-structured-failure":
		inferencePayload := claudeGoldenFailedInferenceObservation(t, events)
		if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("%s inference outcome = %q, want FAILED", scenario.name, inferencePayload.Outcome)
		}
		if inferencePayload.FailureDetail == nil || inferencePayload.FailureDetail.Message != scenario.wantFailure {
			t.Fatalf("%s inference failure detail = %#v, want %q", scenario.name, inferencePayload.FailureDetail, scenario.wantFailure)
		}
		if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
			t.Fatalf("%s structured failure treated COMPLETE-bearing output as success: %q", scenario.name, *inferencePayload.Response)
		}
		assertProviderSessionGoldensMatch(t, *scenario.golden, observeClaudeFailedProviderSessionGoldens(t, inferencePayload, responseEvents))
	case "claude-timeout":
		inferencePayload := claudeGoldenFailedInferenceObservationWithReason(
			t,
			events,
			factoryapi.WorkFailureTypeTimeout,
		)
		if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("%s inference outcome = %q, want FAILED", scenario.name, inferencePayload.Outcome)
		}
		if inferencePayload.FailureDetail == nil || inferencePayload.FailureDetail.Message != scenario.wantFailure {
			t.Fatalf("%s inference failure detail = %#v, want %q", scenario.name, inferencePayload.FailureDetail, scenario.wantFailure)
		}
		if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
			t.Fatalf("%s timeout treated COMPLETE-bearing output as success: %q", scenario.name, *inferencePayload.Response)
		}
		assertClaudeGoldenResponseStreamClosesWithoutSuccess(t, responseEvents)
		// The existing timeout response-event fixture intentionally captures the
		// legacy retry transcript, while this conductor asserts the normalized
		// retry and stream-closure behavior directly above.
	default:
		t.Fatalf("%s has unsupported Claude golden %q", scenario.name, scenario.golden.Manifest.ID)
	}
}
