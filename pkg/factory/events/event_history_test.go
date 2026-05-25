package events

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "codex-cli",
				ModelProvider:    "openai",
				Model:            "gpt-5.4",
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("Workers = %#v, want one runtime worker", payload.Factory.Workers)
	}
	worker := (*payload.Factory.Workers)[0]
	if worker.Name != "builder" || stringValueForEventHistoryTest(worker.ExecutorProvider) != "SCRIPT_WRAP" ||
		stringValueForEventHistoryTest(worker.ModelProvider) != "CODEX" ||
		stringValueForEventHistoryTest(worker.Type) != string(factoryapi.WorkerTypeModelWorker) ||
		stringValueForEventHistoryTest(worker.Model) != "gpt-5.4" {
		t.Fatalf("worker metadata = %#v, want runtime-config provider/model metadata", worker)
	}
}

func TestFactoryEventHistory_SubscribeCancelClosesStreamWithoutPanickingAppenders(t *testing.T) {
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	ctx, cancel := context.WithCancel(context.Background())
	stream := history.Subscribe(ctx)

	history.RecordInitialStructure()

	select {
	case <-stream.Events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial live event")
	}

	cancel()

	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				goto closed
			}
		case <-deadline:
			t.Fatal("timed out waiting for stream closure after cancellation")
		}
	}

closed:
	for i := 0; i < 32; i++ {
		history.RecordFactoryStateChange(i, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning, "post-cancel", time.Unix(int64(i+1), 0).UTC())
	}
}

func TestFactoryEventHistory_RecordInitialStructure_EmitsCanonicalPublicWorkstationKinds(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"Build": {Name: "Build", Kind: interfaces.WorkstationKindRepeater},
			},
		},
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}
	workstation := (*payload.Factory.Workstations)[0]
	if workstation.Behavior == nil || *workstation.Behavior != factoryapi.WorkstationKindRepeater {
		t.Fatalf("workstation behavior = %#v, want REPEATER", workstation.Behavior)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal initial structure event: %v", err)
	}
	if !strings.Contains(string(data), `"behavior":"REPEATER"`) {
		t.Fatalf("initial structure event JSON = %s, want canonical uppercase workstation behavior", data)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_PreservesNonSuccessRouteArrays(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNetWithOrderedNonSuccessRoutes(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}

	workstation := (*payload.Factory.Workstations)[0]
	if workstation.OnContinue == nil || !reflect.DeepEqual(*workstation.OnContinue, []factoryapi.WorkstationIO{{WorkType: "story", State: "retry"}, {WorkType: "story", State: "init"}}) {
		t.Fatalf("workstation onContinue = %#v, want authored route array", workstation.OnContinue)
	}
	if workstation.OnRejection == nil || !reflect.DeepEqual(*workstation.OnRejection, []factoryapi.WorkstationIO{{WorkType: "story", State: "triage"}, {WorkType: "story", State: "init"}}) {
		t.Fatalf("workstation onRejection = %#v, want authored route array", workstation.OnRejection)
	}
	if workstation.OnFailure == nil || !reflect.DeepEqual(*workstation.OnFailure, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}, {WorkType: "story", State: "abandoned"}}) {
		t.Fatalf("workstation onFailure = %#v, want authored route array", workstation.OnFailure)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_ProjectsImplicitCronFailureRoutes(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "cron-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "poll-for-work",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "cron-worker",
			Cron:           &interfaces.CronConfig{Schedule: "* * * * *", TriggerAtStart: true},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}
	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	history := NewFactoryEventHistory(net, func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil {
		t.Fatalf("workstations = %#v, want generated workstations", payload.Factory.Workstations)
	}

	want := []factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}}
	for _, workstation := range *payload.Factory.Workstations {
		if workstation.Name != "poll-for-work" {
			continue
		}
		if workstation.OnFailure == nil || !reflect.DeepEqual(*workstation.OnFailure, want) {
			t.Fatalf("workstation onFailure = %#v, want implicit failed-state route", workstation.OnFailure)
		}
		return
	}
	t.Fatalf("workstations = %#v, want generated cron workstation", payload.Factory.Workstations)
}

func TestFactoryEventHistory_RecordInitialStructure_PreservesGeneratedPublicEnumPointerValues(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             "  MODEL_WORKER  ",
				ExecutorProvider: "  local-claude  ",
				ModelProvider:    "  openai  ",
				Model:            "gpt-5.4",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Build": {
				Name:           "Build",
				Kind:           interfaces.WorkstationKind("  REPEATER  "),
				Type:           "  LOGICAL_MOVE  ",
				WorkerTypeName: "builder",
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("workers = %#v, want one generated worker", payload.Factory.Workers)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}

	worker := (*payload.Factory.Workers)[0]
	if got, want := stringValueForEventHistoryTest(worker.ExecutorProvider), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerProviderPtr(runtimeConfig.Workers["builder"].ExecutorProvider)); got != want {
		t.Fatalf("worker executor provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.ModelProvider), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerModelProviderPtr(runtimeConfig.Workers["builder"].ModelProvider)); got != want {
		t.Fatalf("worker model provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.Type), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerTypePtr(runtimeConfig.Workers["builder"].Type)); got != want {
		t.Fatalf("worker type = %q, want %q", got, want)
	}

	workstation := (*payload.Factory.Workstations)[0]
	if got, want := stringValueForEventHistoryTest(workstation.Type), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkstationTypePtr(runtimeConfig.Workstations["Build"].Type)); got != want {
		t.Fatalf("workstation type = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(workstation.Behavior), stringValueForEventHistoryTest(interfaces.GeneratedPublicWorkstationKindPtr(runtimeConfig.Workstations["Build"].Kind)); got != want {
		t.Fatalf("workstation behavior = %q, want %q", got, want)
	}
}

func TestFactoryEventHistory_RecordDispatchCompletion_PreservesSelectedClassificationLabel(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)

	result := interfaces.WorkResult{
		DispatchID:                  "dispatch-1",
		TransitionID:                "t-review",
		Outcome:                     interfaces.OutcomeAccepted,
		SelectedClassificationLabel: "approved",
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "t-review",
		Outcome:         interfaces.OutcomeAccepted,
		ConsumedTokens:  []interfaces.Token{{ID: "token-1", Color: interfaces.TokenColor{WorkID: "work-1", TraceID: "trace-1"}}},
		OutputMutations: nil,
	}

	history.RecordWorkstationResponse(3, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if got := stringValueForEventHistoryTest(payload.SelectedClassificationLabel); got != "approved" {
		t.Fatalf("selected classification label = %q, want approved", got)
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_UsesContextForRequestIdentity(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationRequest(4, interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-1",
		CreatedTick: 4,
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "build",
			WorkerType:      "builder",
			WorkstationName: "Build",
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-1",
				ReplayKey: "replay-1",
			},
		},
	}, eventTime)

	events := history.Events()
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

func TestFactoryEventHistory_RecordWorkstationResponse_FailedResultIncludesFailureDetails(t *testing.T) {
	eventTime := time.Date(2026, 4, 17, 9, 30, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	result := interfaces.WorkResult{
		DispatchID:   "dispatch-failed",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Output:       "partial output",
		Error:        "provider error: throttled: selected model is at capacity",
		Feedback:     "retry later",
		FailureMetadata: &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyThrottle,
			Type:   interfaces.WorkFailureTypeThrottled,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-failed",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeFailed,
		Reason:          result.Error,
		EndTime:         eventTime,
		Duration:        2 * time.Second,
	}

	history.RecordWorkstationResponse(9, result, completed)

	events := history.Events()
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
	if stringValueForEventHistoryTest(payload.FailureReason) != "throttled" {
		t.Fatalf("failure reason = %q, want throttled", stringValueForEventHistoryTest(payload.FailureReason))
	}
	if stringValueForEventHistoryTest(payload.FailureMessage) != result.Error {
		t.Fatalf("failure message = %q, want %q", stringValueForEventHistoryTest(payload.FailureMessage), result.Error)
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
	assertJSONField(t, payloadObject, "failureReason", "throttled")
	assertJSONField(t, payloadObject, "failureMessage", result.Error)
	providerFailure := assertJSONObject(t, payloadObject, "providerFailure")
	assertJSONField(t, providerFailure, "family", "throttle")
	assertJSONField(t, providerFailure, "type", "throttled")
}

func TestFactoryEventHistory_RecordWorkstationResponse_CodexWindowsExitCode4294967295UsesRetryableProviderFailureMetadata(t *testing.T) {
	eventTime := time.Date(2026, 4, 21, 1, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	errorText := "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	result := interfaces.WorkResult{
		DispatchID:   "dispatch-codex-windows-4294967295",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Error:        errorText,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeInternalServerError,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      result.DispatchID,
		TransitionID:    result.TransitionID,
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeFailed,
		Reason:          errorText,
		EndTime:         eventTime,
		Duration:        3 * time.Second,
	}

	history.RecordWorkstationResponse(12, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForEventHistoryTest(payload.FailureReason) != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("failure reason = %q, want %q", stringValueForEventHistoryTest(payload.FailureReason), interfaces.ProviderErrorTypeInternalServerError)
	}
	if stringValueForEventHistoryTest(payload.FailureMessage) != errorText {
		t.Fatalf("failure message = %q, want %q", stringValueForEventHistoryTest(payload.FailureMessage), errorText)
	}
	if payload.ProviderFailure == nil {
		t.Fatal("expected provider failure metadata on dispatch completed payload")
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Family) != string(interfaces.ProviderErrorFamilyRetryable) {
		t.Fatalf("provider failure family = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Family), interfaces.ProviderErrorFamilyRetryable)
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Type) != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("provider failure type = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Type), interfaces.ProviderErrorTypeInternalServerError)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_OmitsRetiredProviderAttemptFields(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 10, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationResponse(12, safeDiagnosticsWorkResult(), safeDiagnosticsCompletedDispatch(eventTime))

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertThinDispatchResponseSerializedEvent(t, events[0])
}

func safeDiagnosticsWorkResult() interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   "dispatch-diagnostics",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "completed",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-123",
		},
		Diagnostics: &interfaces.WorkDiagnostics{
			RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
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
			Provider: &interfaces.ProviderDiagnostic{
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
			Command: &interfaces.CommandDiagnostic{
				Stdin: "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &interfaces.PanicDiagnostic{Stack: "panic stack should not be dashboard-facing"},
		},
	}
}

func safeDiagnosticsCompletedDispatch(eventTime time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-diagnostics",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeAccepted,
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

func TestFailureDetailsForResult_NonFailedResultsOmitFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-rejected",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeRejected,
		Feedback:     "needs revision",
	})

	if reason != "" || message != "" {
		t.Fatalf("failure details = %q/%q, want empty for rejected result", reason, message)
	}
}

func TestFailureDetailsForResult_FailedWorkerErrorUsesStableFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-worker-error",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "script exited with code 1",
	})

	if reason != failureReasonWorkerError {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonWorkerError)
	}
	if message != "script exited with code 1" {
		t.Fatalf("failure message = %q, want script error", message)
	}
}

func TestFailureDetailsForResult_FailureMetadataOverridesWorkerErrorReason(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:      "dispatch-timeout",
		TransitionID:    "build",
		Outcome:         interfaces.OutcomeFailed,
		Error:           "provider error: timeout: context deadline exceeded",
		FailureMetadata: &interfaces.WorkFailureMetadata{Type: interfaces.WorkFailureTypeTimeout},
	})

	if reason != string(interfaces.WorkFailureTypeTimeout) {
		t.Fatalf("failure reason = %q, want %q", reason, interfaces.WorkFailureTypeTimeout)
	}
	if message != "provider error: timeout: context deadline exceeded" {
		t.Fatalf("failure message = %q, want preserved rendered timeout text", message)
	}
}

func TestFailureDetailsForResult_ClassifierInvalidOutputPreservesRawOutputEvidence(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-classifier-invalid",
		TransitionID: "classify",
		Outcome:      interfaces.OutcomeFailed,
		Error:        `classifier output invalid: expected plain string label (raw output: "{\"label\":\"approved\"}")`,
	})

	if reason != failureReasonWorkerError {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonWorkerError)
	}
	if !strings.Contains(message, `raw output: "{\"label\":\"approved\"}"`) {
		t.Fatalf("failure message = %q, want raw output evidence", message)
	}
}

func TestFailureDetailsForResult_FailedWithoutDetailsUsesUnavailableMessage(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-unknown",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
	})

	if reason != failureReasonUnknown {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonUnknown)
	}
	if message != failureMessageUnavailable {
		t.Fatalf("failure message = %q, want unavailable message", message)
	}
}

type eventHistoryRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func eventHistoryProjectionNet() *state.Net {
	return &state.Net{
		ID: "event-history-projection-net",
		Places: map[string]*petri.Place{
			"story:init":   {ID: "story:init", TypeID: "story", State: "init"},
			"story:review": {ID: "story:review", TypeID: "story", State: "review"},
			"story:done":   {ID: "story:done", TypeID: "story", State: "done"},
			"story:failed": {ID: "story:failed", TypeID: "story", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs:  []petri.Arc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []petri.Arc{{PlaceID: "story:review"}},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func eventHistoryProjectionNetWithOrderedNonSuccessRoutes() *state.Net {
	return &state.Net{
		ID: "event-history-projection-net-non-success-routes",
		Places: map[string]*petri.Place{
			"story:init":      {ID: "story:init", TypeID: "story", State: "init"},
			"story:review":    {ID: "story:review", TypeID: "story", State: "review"},
			"story:retry":     {ID: "story:retry", TypeID: "story", State: "retry"},
			"story:triage":    {ID: "story:triage", TypeID: "story", State: "triage"},
			"story:failed":    {ID: "story:failed", TypeID: "story", State: "failed"},
			"story:abandoned": {ID: "story:abandoned", TypeID: "story", State: "abandoned"},
		},
		Transitions: map[string]*petri.Transition{
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs:  []petri.Arc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []petri.Arc{{PlaceID: "story:review"}},
				ContinueArcs: []petri.Arc{
					{PlaceID: "story:retry"},
					{PlaceID: "story:init"},
				},
				RejectionArcs: []petri.Arc{
					{PlaceID: "story:triage"},
					{PlaceID: "story:init"},
				},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
					{PlaceID: "story:abandoned"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "retry", Category: state.StateCategoryProcessing},
					{Value: "triage", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
					{Value: "abandoned", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func stringValueForEventHistoryTest[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSliceValueForEventHistoryTest(value *[]string) []string {
	if value == nil {
		return nil
	}
	out := make([]string, len(*value))
	copy(out, *value)
	return out
}
