package events

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func assertJSONField(t *testing.T, object map[string]any, field string, want any) {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON field %q in %#v", field, object)
	}
	if got != want {
		t.Fatalf("JSON field %q = %#v, want %#v", field, got, want)
	}
}

func assertJSONObject(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON object field %q in %#v", field, object)
	}
	value, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("JSON field %q = %#v, want object", field, got)
	}
	return value
}

func TestFactoryEventHistory_RecordWorkstationRequest_UsesContextForRequestIdentity(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationRequest(4, interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-1",
		CreatedTick: 4,
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "build",
			WorkerType:      "builder",
			WorkstationName: "Build",
			Execution: work.ExecutionMetadata{
				RequestID: "request-1",
				ReplayKey: "replay-1",
			},
		},
	}, eventTime)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical events: %v", err)
	}
	if len(stream.History) != 1 {
		t.Fatalf("canonical history count = %d, want 1", len(stream.History))
	}
	var canonicalPayload interfaces.DispatchRequestEventPayload
	if err := stream.History[0].DecodePayload(&canonicalPayload); err != nil {
		t.Fatalf("decode canonical dispatch request payload: %v", err)
	}
	if canonicalPayload.Metadata == nil || stringValueForEventHistoryTest(canonicalPayload.Metadata.ReplayKey) != "replay-1" {
		t.Fatalf("canonical metadata = %#v, want replay-1", canonicalPayload.Metadata)
	}

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeDispatchRequest {
		t.Fatalf("event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeDispatchRequest)
	}

	payload, err := events[0].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if stringValueForEventHistoryTest(events[0].Context.RequestId) != "request-1" {
		t.Fatalf("context requestId = %q, want request-1", stringValueForEventHistoryTest(events[0].Context.RequestId))
	}
	if payload.Metadata == nil {
		t.Fatal("metadata = nil, want replay metadata object")
	}
	if stringValueForEventHistoryTest(payload.Metadata.ReplayKey) != "replay-1" {
		t.Fatalf("metadata replayKey = %q, want replay-1", stringValueForEventHistoryTest(payload.Metadata.ReplayKey))
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_NormalizesEventTimeToUTC(t *testing.T) {
	localZone := time.FixedZone("Factory/Local", 7*60*60)
	eventTime := time.Date(2026, 4, 22, 23, 30, 0, 0, localZone)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationRequest(4, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-utc",
		Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-utc",
			TransitionID: "build",
		},
	}, eventTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-22T16:30:00Z")
}

func TestFactoryEventHistory_RecordWorkstationRequest_UsesFactoryRunnerOverrideMetadata(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Factory: &interfaces.FactoryConfig{Runner: "codex"},
		},
	)
	history.SetFactoryRunnerOverride("  gemini  ")

	metadata := dispatchRequestMetadataForEventHistoryTest(t, history)
	if got := stringValueForEventHistoryTest(metadata.RunnerId); got != workerexecution.RunnerIDGemini {
		t.Fatalf("metadata runnerId = %q, want %q", got, workerexecution.RunnerIDGemini)
	}
	if got := stringValueForEventHistoryTest(metadata.RunnerSelectionSource); got != string(workerexecution.RunnerSelectionSourceFactory) {
		t.Fatalf("metadata runnerSelectionSource = %q, want %q", got, workerexecution.RunnerSelectionSourceFactory)
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_UsesSharedFactoryConfigRunnerMetadata(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Factory: &interfaces.FactoryConfig{Runner: "  opencode  "},
		},
	)

	metadata := dispatchRequestMetadataForEventHistoryTest(t, history)
	if got := stringValueForEventHistoryTest(metadata.RunnerId); got != workerexecution.RunnerIDOpenCode {
		t.Fatalf("metadata runnerId = %q, want %q", got, workerexecution.RunnerIDOpenCode)
	}
	if got := stringValueForEventHistoryTest(metadata.RunnerSelectionSource); got != string(workerexecution.RunnerSelectionSourceFactory) {
		t.Fatalf("metadata runnerSelectionSource = %q, want %q", got, workerexecution.RunnerSelectionSourceFactory)
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_DefaultsRunnerMetadataWithoutFactoryConfigCapability(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryDefinitionOnlyRuntimeConfig{},
	)

	metadata := dispatchRequestMetadataForEventHistoryTest(t, history)
	if got := stringValueForEventHistoryTest(metadata.RunnerId); got != workerexecution.RunnerIDCodex {
		t.Fatalf("metadata runnerId = %q, want %q", got, workerexecution.RunnerIDCodex)
	}
	if got := stringValueForEventHistoryTest(metadata.RunnerSelectionSource); got != string(workerexecution.RunnerSelectionSourceDefault) {
		t.Fatalf("metadata runnerSelectionSource = %q, want %q", got, workerexecution.RunnerSelectionSourceDefault)
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_DefaultsRunnerMetadataWhenFactoryConfigNil(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{},
	)

	metadata := dispatchRequestMetadataForEventHistoryTest(t, history)
	if got := stringValueForEventHistoryTest(metadata.RunnerId); got != workerexecution.RunnerIDCodex {
		t.Fatalf("metadata runnerId = %q, want %q", got, workerexecution.RunnerIDCodex)
	}
	if got := stringValueForEventHistoryTest(metadata.RunnerSelectionSource); got != string(workerexecution.RunnerSelectionSourceDefault) {
		t.Fatalf("metadata runnerSelectionSource = %q, want %q", got, workerexecution.RunnerSelectionSourceDefault)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_FailedResultIncludesFailureDetails(t *testing.T) {
	eventTime := time.Date(2026, 4, 17, 9, 30, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	result := workerexecution.WorkResult{
		DispatchID:   "dispatch-failed",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeFailed,
		Output:       "partial output",
		Error:        "provider error: throttled: selected model is at capacity",
		Feedback:     "retry later",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyThrottle,
			Type:   workerexecution.WorkFailureTypeThrottled,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-failed",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         workerexecution.OutcomeFailed,
		Reason:          result.Error,
		EndTime:         eventTime,
		Duration:        2 * time.Second,
	}

	history.RecordWorkstationResponse(9, result, completed)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical events: %v", err)
	}
	if len(stream.History) != 1 {
		t.Fatalf("canonical history count = %d, want 1", len(stream.History))
	}
	var canonicalPayload workerexecution.DispatchResponseEventPayload
	if err := stream.History[0].DecodePayload(&canonicalPayload); err != nil {
		t.Fatalf("decode canonical dispatch response payload: %v", err)
	}
	if canonicalPayload.FailureDetail == nil || canonicalPayload.FailureDetail.Reason != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("canonical failure detail = %#v, want throttled", canonicalPayload.FailureDetail)
	}

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeDispatchResponse {
		t.Fatalf("event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeDispatchResponse)
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if payload.FailureDetail == nil || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeThrottled {
		t.Fatalf("failure detail = %#v, want throttled", payload.FailureDetail)
	}
	if payload.FailureDetail.Message != result.Error {
		t.Fatalf("failure message = %q, want %q", payload.FailureDetail.Message, result.Error)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payloadObject := assertJSONObject(t, decoded, "payload")
	assertJSONField(t, payloadObject, "outcome", "FAILED")
	assertJSONField(t, payloadObject, "output", "partial output")
	assertJSONField(t, payloadObject, "error", result.Error)
	assertJSONField(t, payloadObject, "feedback", "retry later")
	failureDetail := assertJSONObject(t, payloadObject, "failureDetail")
	assertJSONField(t, failureDetail, "reason", "throttled")
	assertJSONField(t, failureDetail, "message", result.Error)
	providerFailure := assertJSONObject(t, payloadObject, "providerFailure")
	assertJSONField(t, providerFailure, "family", "throttle")
	assertJSONField(t, providerFailure, "type", "throttled")
}

func TestFactoryEventHistory_RecordWorkstationResponse_UsesUTCFallbackAndDurationMillisForMissingEndTime(t *testing.T) {
	localZone := time.FixedZone("Factory/Local", -5*60*60)
	fallbackTime := time.Date(2026, 4, 17, 9, 30, 0, 0, localZone)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return fallbackTime })
	result := workerexecution.WorkResult{
		DispatchID:   "dispatch-fallback",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeAccepted,
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:   result.DispatchID,
		TransitionID: result.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Duration:     1500 * time.Millisecond,
	}

	history.RecordWorkstationResponse(9, result, completed)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-17T14:30:00Z")
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if payload.DurationMillis == nil || *payload.DurationMillis != 1500 {
		t.Fatalf("durationMillis = %#v, want 1500", payload.DurationMillis)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_CodexWindowsExitCode4294967295UsesRetryableProviderFailureMetadata(t *testing.T) {
	eventTime := time.Date(2026, 4, 21, 1, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	errorText := "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	result := workerexecution.WorkResult{
		DispatchID:   "dispatch-codex-windows-4294967295",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        errorText,
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      result.DispatchID,
		TransitionID:    result.TransitionID,
		WorkstationName: "Build",
		Outcome:         workerexecution.OutcomeFailed,
		Reason:          errorText,
		EndTime:         eventTime,
		Duration:        3 * time.Second,
	}

	history.RecordWorkstationResponse(12, result, completed)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if payload.FailureDetail == nil || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeInternalServerError {
		t.Fatalf("failure detail = %#v, want %q", payload.FailureDetail, workerexecution.WorkFailureTypeInternalServerError)
	}
	if payload.FailureDetail.Message != errorText {
		t.Fatalf("failure message = %q, want %q", payload.FailureDetail.Message, errorText)
	}
	if payload.ProviderFailure == nil {
		t.Fatal("expected provider failure metadata on dispatch completed payload")
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Family) != string(workerexecution.WorkFailureFamilyRetryable) {
		t.Fatalf("provider failure family = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Family), workerexecution.WorkFailureFamilyRetryable)
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Type) != string(workerexecution.WorkFailureTypeInternalServerError) {
		t.Fatalf("provider failure type = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Type), workerexecution.WorkFailureTypeInternalServerError)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payloadObject := assertJSONObject(t, decoded, "payload")
	providerFailure := assertJSONObject(t, payloadObject, "providerFailure")
	assertJSONField(t, providerFailure, "family", "retryable")
	assertJSONField(t, providerFailure, "type", "internal_server_error")
}

func TestFactoryEventHistory_RecordWorkstationResponse_OmitsRetiredProviderAttemptFields(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 10, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationResponse(12, safeDiagnosticsWorkResult(), safeDiagnosticsCompletedDispatch(eventTime))

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertThinDispatchResponseSerializedEvent(t, events[0])
}

func assertEventTimeUTCJSON(t *testing.T, event factoryapi.FactoryEvent, want string) {
	t.Helper()
	if event.Context.EventTime.Location() != time.UTC {
		t.Fatalf("event time location = %v, want UTC", event.Context.EventTime.Location())
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if !strings.Contains(string(data), `"eventTime":"`+want+`"`) {
		t.Fatalf("event JSON = %s, want eventTime %q", data, want)
	}
}

func safeDiagnosticsWorkResult() workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   "dispatch-diagnostics",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "completed",
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-123",
		},
		Diagnostics: &workerexecution.WorkDiagnostics{
			RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
				SystemPromptHash: "system-hash-123",
				UserMessageHash:  "user-hash-456",
				Variables: map[string]string{
					"prompt_source":  "factory-renderer",
					"work_type_name": "story",
					"system_prompt":  "raw rendered system prompt must stay private",
					"user_message":   "raw rendered user message must stay private",
					"stdin":          "raw rendered stdin must stay private",
					"env":            "raw rendered environment must stay private",
				},
			},
			Provider: &workerexecution.ProviderDiagnostic{
				Provider: "codex",
				Model:    "gpt-5.4",
				RequestMetadata: map[string]string{
					"prompt_source":       "provider-renderer",
					"worker_type":         "builder",
					"system_prompt":       "raw system prompt must stay private",
					"raw_system_prompt":   "raw variant system prompt must stay private",
					"system_prompt_body":  "raw prompt body must stay private",
					"user_message_text":   "raw user message text must stay private",
					"stdin_payload":       "raw stdin payload must stay private",
					"env_secret":          "raw env secret must stay private",
					"unreviewed_metadata": "unreviewed provider metadata must stay private",
				},
				ResponseMetadata: map[string]string{
					"retry_count":         "1",
					"provider_session_id": "resp-safe-123",
					"system_prompt_body":  "raw response prompt body must stay private",
					"user_message_text":   "raw response user message text must stay private",
					"stdin_payload":       "raw response stdin payload must stay private",
					"env_secret":          "raw response env secret must stay private",
				},
			},
			Command: &workerexecution.CommandDiagnostic{
				Stdin: "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &workerexecution.PanicDiagnostic{Stack: "panic stack should not be dashboard-facing"},
		},
	}
}

func safeDiagnosticsCompletedDispatch(eventTime time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-diagnostics",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         workerexecution.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        3 * time.Second,
	}
}

func assertThinDispatchResponseSerializedEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	body := string(data)
	for _, unsafe := range unsafeDiagnosticEventValues() {
		if strings.Contains(body, unsafe) {
			t.Fatalf("event JSON leaked unsafe diagnostic value %q: %s", unsafe, body)
		}
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payload := assertJSONObject(t, decoded, "payload")
	for _, retiredField := range []string{"inputs", "providerSession", "diagnostics"} {
		if _, ok := payload[retiredField]; ok {
			t.Fatalf("dispatch response payload must not serialize retired %q: %#v", retiredField, payload)
		}
	}
}

func unsafeDiagnosticEventValues() []string {
	return []string{
		"raw system prompt must stay private",
		"raw variant system prompt must stay private",
		"raw prompt body must stay private",
		"raw user message text must stay private",
		"raw stdin payload must stay private",
		"raw env secret must stay private",
		"unreviewed provider metadata must stay private",
		"raw response prompt body must stay private",
		"raw response user message text must stay private",
		"raw response stdin payload must stay private",
		"raw response env secret must stay private",
		"raw rendered system prompt must stay private",
		"raw rendered user message must stay private",
		"raw rendered stdin must stay private",
		"raw rendered environment must stay private",
		"raw command stdin must stay private",
		"raw environment value must stay private",
		"AGENT_FACTORY_AUTH_TOKEN",
		"panic stack should not be dashboard-facing",
	}
}

func dispatchRequestMetadataForEventHistoryTest(t *testing.T, history *FactoryEventHistory) *factoryapi.DispatchRequestEventMetadata {
	t.Helper()
	history.RecordWorkstationRequest(4, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-runner",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-runner",
			TransitionID:    "build",
			WorkerType:      "builder",
			WorkstationName: "Build",
			Execution: work.ExecutionMetadata{
				ReplayKey: "replay-runner",
			},
		},
	}, time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC))

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if payload.Metadata == nil {
		t.Fatal("metadata = nil, want dispatch metadata")
	}
	return payload.Metadata
}
