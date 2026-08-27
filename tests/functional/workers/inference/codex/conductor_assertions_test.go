package codex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertCodexWork(
	t *testing.T,
	scenario codexConductorScenario,
	listed factoryapi.ListWorkResponse,
) {
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

func assertCodexDispatch(
	t *testing.T,
	scenario codexConductorScenario,
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
		if scenario.wantFailure != "" {
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

func assertCodexCommand(
	t *testing.T,
	router *codexCommandRouter,
	scenario codexConductorScenario,
) {
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
		if request.Command != codexConductorProcessCommand {
			t.Fatalf("%s command = %q, want codex", scenario.name, request.Command)
		}
		if request.WorkDir != scenario.factoryDir {
			t.Fatalf("%s command WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
		}
		if !containsArgPair(request.Args, "--model", scenario.model) {
			t.Fatalf("%s args = %#v, want --model %s", scenario.name, request.Args, scenario.model)
		}
		if !containsArg(request.Args, "exec") || !containsArg(request.Args, "--json") {
			t.Fatalf("%s args = %#v, want codex exec --json streaming invocation", scenario.name, request.Args)
		}
	}
}

func assertCodexProviderSession(
	t *testing.T,
	scenario codexConductorScenario,
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

func assertCodexEventScope(
	t *testing.T,
	scenario codexConductorScenario,
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
	if scenario.wantFailure == "" {
		return
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("%s marshal Factory Events: %v", scenario.name, err)
	}
	text := string(payload)
	if !strings.Contains(text, scenario.wantFailure) {
		t.Fatalf("%s Factory Events missing expected failure %q: %s", scenario.name, scenario.wantFailure, text)
	}
	if strings.Contains(text, "Codex command did not complete successfully") {
		t.Fatalf("%s Factory Events used Codex-local cancellation fallback: %s", scenario.name, text)
	}
}

func assertCodexResponseEvents(
	t *testing.T,
	scenario codexConductorScenario,
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

func readCodexResponseEventsUntilTerminal(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	timeout time.Duration,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	events := make([]factoryapi.FactoryResponseEvent, 0, 8)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for terminal Codex response event after %s; got %d events", timeout, len(events))
		}
		result := stream.TryNextFrameResult(remaining)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("Codex response stream ended before terminal event: %s", result.Diagnostic())
		}
		event := result.Frame.Event
		events = append(events, event)
		if isCodexTerminalResponseEvent(event) {
			return events
		}
	}
}

func isCodexTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
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
