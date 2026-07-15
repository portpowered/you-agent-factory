package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestFactoryEventHistory_RecordAgentRunEvent_OwnsEnvelopeAndPreservesPublicPayload(t *testing.T) {
	eventTime := time.Date(2026, 7, 15, 22, 30, 0, 123456789, time.FixedZone("offset", -7*60*60))
	diagnostics := json.RawMessage(`{"agentRun":{"executionBehavior":"agent_loop","transcript":[{"role":"assistant","summary":"done"}]}}`)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordAgentRunEvent(workerexecution.AgentRunResponseEvent{
		ID:         "factory-event/agent-run-response/dispatch-agent",
		DispatchID: "dispatch-agent",
		EventTime:  eventTime,
		Payload: workerexecution.AgentRunResponseEventPayload{
			AgentRunID:     "dispatch-agent/agent-run/1",
			Diagnostics:    diagnostics,
			DurationMillis: 1250,
			Outcome:        string(workerexecution.OutcomeAccepted),
		},
	})

	canonical := history.CanonicalEvents()
	if len(canonical) != 1 || canonical[0].Type != interfaces.FactoryEventTypeAgentRunResponse {
		t.Fatalf("canonical events = %#v, want one agent-run response", canonical)
	}
	if canonical[0].Context.EventTime.Location() != time.UTC || !canonical[0].Context.EventTime.Equal(eventTime) {
		t.Fatalf("event time = %s (%s), want same instant normalized to UTC", canonical[0].Context.EventTime, canonical[0].Context.EventTime.Location())
	}
	if canonical[0].Context.DispatchID == nil || *canonical[0].Context.DispatchID != "dispatch-agent" {
		t.Fatalf("dispatch ID = %#v, want dispatch-agent", canonical[0].Context.DispatchID)
	}

	publicEvents := history.Events()
	payload, err := publicEvents[0].Payload.AsAgentRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode public agent-run payload: %v", err)
	}
	if payload.AgentRunId != "dispatch-agent/agent-run/1" || payload.DurationMillis != 1250 || payload.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("public payload = %#v, want preserved agent-run result", payload)
	}
	if payload.Diagnostics == nil || payload.Diagnostics.AgentRun == nil || len(*payload.Diagnostics.AgentRun.Transcript) != 1 {
		t.Fatalf("public diagnostics = %#v, want bounded agent-run transcript", payload.Diagnostics)
	}
}

func TestFactoryEventHistory_RecordScriptEvent_AppendsScriptBoundaryEvents(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 14, 5, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	scriptRequestID := "dispatch-script/script-request/1"

	recordScriptBoundaryEvents(history, eventTime, scriptRequestID)

	events := history.Events()
	assertRecordedScriptBoundaryEvents(t, events)
	assertRecordedScriptRequestPayload(t, events[0], scriptRequestID)
	assertRecordedScriptResponsePayload(t, events[1], scriptRequestID)
}

func TestFactoryEventHistory_RecordScriptEvent_IgnoresNonScriptEvents(t *testing.T) {
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordScriptEvent(workerexecution.ScriptEvent{
		ID:         "factory-event/script-request/invalid",
		Kind:       workerexecution.ScriptEventKindRequest,
		EventTime:  time.Unix(0, 0).UTC(),
		DispatchID: "dispatch-script",
		Response:   &workerexecution.ScriptResponseEventPayload{},
	})

	if events := history.Events(); len(events) != 0 {
		t.Fatalf("event count = %d, want 0 when script recorder receives non-script event", len(events))
	}
}

func recordScriptBoundaryEvents(history *FactoryEventHistory, eventTime time.Time, scriptRequestID string) {
	base := workerexecution.ScriptEvent{
		EventTime:  eventTime,
		Tick:       14,
		DispatchID: "dispatch-script",
		RequestID:  "request-script",
		TraceIDs:   []string{"trace-script"},
		WorkIDs:    []string{"work-script-1", "work-script-2"},
	}
	request := base
	request.ID = "factory-event/script-request/dispatch-script/1"
	request.Kind = workerexecution.ScriptEventKindRequest
	request.Request = &workerexecution.ScriptRequestEventPayload{
		ScriptRequestID: scriptRequestID,
		DispatchID:      "dispatch-script",
		TransitionID:    "build",
		Attempt:         1,
		Command:         "python",
		Args:            []string{"main.py", "--mode", "review"},
	}
	history.RecordScriptEvent(request)

	response := base
	response.ID = "factory-event/script-response/dispatch-script/1"
	response.Kind = workerexecution.ScriptEventKindResponse
	response.Response = &workerexecution.ScriptResponseEventPayload{
		ScriptRequestID: scriptRequestID,
		DispatchID:      "dispatch-script",
		TransitionID:    "build",
		Attempt:         1,
		Outcome:         workerexecution.ScriptExecutionOutcomeSucceeded,
		Stdout:          "ok",
		Stderr:          "",
		DurationMillis:  1250,
	}
	history.RecordScriptEvent(response)
}

func assertRecordedScriptBoundaryEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeScriptRequest {
		t.Fatalf("first event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeScriptRequest)
	}
	if events[1].Type != factoryapi.FactoryEventTypeScriptResponse {
		t.Fatalf("second event type = %s, want %s", events[1].Type, factoryapi.FactoryEventTypeScriptResponse)
	}
	if events[0].Id != "factory-event/script-request/dispatch-script/1" {
		t.Fatalf("script request event id = %q, want stable request id", events[0].Id)
	}
	if events[1].Id != "factory-event/script-response/dispatch-script/1" {
		t.Fatalf("script response event id = %q, want stable response id", events[1].Id)
	}
	if events[0].Context.Sequence != 0 || events[1].Context.Sequence != 1 {
		t.Fatalf("event sequences = %d/%d, want 0/1", events[0].Context.Sequence, events[1].Context.Sequence)
	}
	if stringValueForEventHistoryTest(events[0].Context.DispatchId) != "dispatch-script" ||
		stringValueForEventHistoryTest(events[0].Context.RequestId) != "request-script" {
		t.Fatalf("script request context = %#v, want canonical dispatch/request correlation", events[0].Context)
	}
	if got := stringSliceValueForEventHistoryTest(events[0].Context.TraceIds); len(got) != 1 || got[0] != "trace-script" {
		t.Fatalf("trace IDs = %#v, want canonical trace correlation", got)
	}
	if got := stringSliceValueForEventHistoryTest(events[0].Context.WorkIds); len(got) != 2 || got[0] != "work-script-1" || got[1] != "work-script-2" {
		t.Fatalf("work IDs = %#v, want canonical work correlation", got)
	}
}

func assertRecordedScriptRequestPayload(t *testing.T, event factoryapi.FactoryEvent, scriptRequestID string) {
	t.Helper()

	requestPayload, err := event.Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("script request payload: %v", err)
	}
	if requestPayload.ScriptRequestId != scriptRequestID ||
		requestPayload.DispatchId != "dispatch-script" ||
		requestPayload.TransitionId != "build" ||
		requestPayload.Attempt != 1 ||
		requestPayload.Command != "python" ||
		strings.Join(requestPayload.Args, ",") != "main.py,--mode,review" {
		t.Fatalf("script request payload = %#v, want canonical request fields", requestPayload)
	}
}

func assertRecordedScriptResponsePayload(t *testing.T, event factoryapi.FactoryEvent, scriptRequestID string) {
	t.Helper()

	responsePayload, err := event.Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("script response payload: %v", err)
	}
	if responsePayload.ScriptRequestId != scriptRequestID ||
		responsePayload.DispatchId != "dispatch-script" ||
		responsePayload.TransitionId != "build" ||
		responsePayload.Attempt != 1 ||
		responsePayload.Outcome != factoryapi.ScriptExecutionOutcomeSucceeded ||
		responsePayload.Stdout != "ok" ||
		responsePayload.Stderr != "" ||
		responsePayload.DurationMillis != 1250 {
		t.Fatalf("script response payload = %#v, want canonical response fields", responsePayload)
	}
}
