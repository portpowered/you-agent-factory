package events

import (
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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

	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeInferenceRequest,
		"factory-event/inference-request/dispatch-script/1",
		factoryapi.FactoryEventContext{
			Tick:       1,
			EventTime:  time.Unix(0, 0).UTC(),
			DispatchId: stringPtr("dispatch-script"),
		},
		factoryapi.InferenceRequestEventPayload{
			InferenceRequestId: "dispatch-script/inference-request/1",
			Attempt:            1,
			WorkingDirectory:   "/tmp/ignored",
			Worktree:           "/tmp/ignored/worktree",
			Prompt:             "ignored",
		},
	))

	if events := history.Events(); len(events) != 0 {
		t.Fatalf("event count = %d, want 0 when script recorder receives non-script event", len(events))
	}
}

func recordScriptBoundaryEvents(history *FactoryEventHistory, eventTime time.Time, scriptRequestID string) {
	context := factoryapi.FactoryEventContext{
		Tick:       14,
		EventTime:  eventTime,
		DispatchId: stringPtr("dispatch-script"),
		RequestId:  stringPtr("request-script"),
		TraceIds:   stringSlicePtr([]string{"trace-script"}),
		WorkIds:    stringSlicePtr([]string{"work-script-1", "work-script-2"}),
	}

	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeScriptRequest,
		"factory-event/script-request/dispatch-script/1",
		context,
		factoryapi.ScriptRequestEventPayload{
			ScriptRequestId: scriptRequestID,
			DispatchId:      "dispatch-script",
			TransitionId:    "build",
			Attempt:         1,
			Command:         "python",
			Args:            []string{"main.py", "--mode", "review"},
		},
	))
	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeScriptResponse,
		"factory-event/script-response/dispatch-script/1",
		context,
		factoryapi.ScriptResponseEventPayload{
			ScriptRequestId: scriptRequestID,
			DispatchId:      "dispatch-script",
			TransitionId:    "build",
			Attempt:         1,
			Outcome:         factoryapi.ScriptExecutionOutcomeSucceeded,
			Stdout:          "ok",
			Stderr:          "",
			DurationMillis:  1250,
		},
	))
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
