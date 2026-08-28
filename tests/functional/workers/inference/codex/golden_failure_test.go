package codex

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexGoldenStructuredFailureCase = "structured-failure"
	codexGoldenTimeoutCase           = "timeout"
)

func assertCodexGoldenStructuredFailureOutcome(t *testing.T, replay codexGoldenReplayResult) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, replay.Listed)
	}
	if replay.Runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", replay.Runner.CallCount())
	}

	inference := failedInferenceResponse(t, replay.FactoryEvents)
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inference.Outcome)
	}
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want structured failure not timeout", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Reason != factoryapi.WorkFailureTypeAuthFailure {
		t.Fatalf("failure reason = %q, want auth_failure", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Message != "Codex authentication failed." {
		t.Fatalf("failure message = %q, want Codex authentication failed.", inference.FailureDetail.Message)
	}

	observed := observeCodexStructuredFailureGoldens(t, replay, inference)
	assertCodexGoldenGoldensMatch(t, replay.Loaded, observed)
}

func assertCodexGoldenTimeoutOutcome(t *testing.T, replay codexGoldenReplayResult) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, replay.Listed)
	}
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, replay.Listed)
	}
	if replay.Runner.CallCount() < 1 {
		t.Fatalf("provider command runner calls = %d, want at least 1", replay.Runner.CallCount())
	}

	inference := failedInferenceResponseWithReason(t, replay.FactoryEvents, factoryapi.WorkFailureTypeTimeout)
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inference.Outcome)
	}
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want TIMEOUT", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Message != "provider invocation timed out" {
		t.Fatalf("failure message = %q, want provider invocation timed out", inference.FailureDetail.Message)
	}
	assertCodexGoldenTimeoutHasNoFalseTerminalSuccess(t, replay, inference)

	observed := observeCodexTimeoutGoldens(t, replay, inference)
	assertCodexGoldenGoldensMatch(t, replay.Loaded, observed)
}

func observeCodexStructuredFailureGoldens(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSession := projectCodexFailureProviderSessionGolden(replay.Loaded, inference)
	responseEvents := projectCodexFailureResponseEventGoldens(t, replay.ResponseEvents)
	invocationResult := projectCodexFailureInvocationResultGolden(t, inference)

	sessionBytes, err := json.Marshal(providerSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}
	resultBytes, err := json.Marshal(invocationResult)
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:  sessionBytes,
		ResponseEvents:   responseEvents,
		InvocationResult: resultBytes,
	}
}

func observeCodexTimeoutGoldens(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSession := projectCodexFailureProviderSessionGolden(replay.Loaded, inference)
	responseEvents := projectCodexFailureResponseEventGoldens(t, replay.ResponseEvents)
	invocationResult := projectCodexFailureInvocationResultGolden(t, inference)

	sessionBytes, err := json.Marshal(providerSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}
	resultBytes, err := json.Marshal(invocationResult)
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:  sessionBytes,
		ResponseEvents:   responseEvents,
		InvocationResult: resultBytes,
	}
}

func failedInferenceResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var terminal factoryapi.InferenceResponseEventPayload
	found := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		terminal = payload
		found = true
	}
	if !found {
		t.Fatal("missing failed inference response")
	}
	return terminal
}

func failedInferenceResponseWithReason(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantReason factoryapi.WorkFailureType,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	inference := failedInferenceResponse(t, events)
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason != wantReason {
		t.Fatalf("failure reason = %q, want %q", inference.FailureDetail.Reason, wantReason)
	}
	return inference
}

func assertCodexGoldenTimeoutHasNoFalseTerminalSuccess(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) {
	t.Helper()

	var lastInference factoryapi.InferenceResponseEventPayload
	foundInference := false
	for _, event := range replay.FactoryEvents {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		lastInference = payload
		foundInference = true
	}
	if !foundInference {
		t.Fatal("missing public inference observations")
	}
	if lastInference.Outcome == factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf(
			"terminal inference outcome = %q, must not invent successful terminal inference on timeout",
			lastInference.Outcome,
		)
	}
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("terminal failure inference outcome = %q, want FAILED", inference.Outcome)
	}

	for _, event := range replay.ResponseEvents {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}
		if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
		if err != nil {
			t.Fatalf("decode message response event: %v", err)
		}
		for _, block := range payload.ContentBlocks {
			text, err := block.AsFactoryResponseEventTextContentBlock()
			if err != nil {
				continue
			}
			if strings.Contains(text.Text, "COMPLETE") {
				t.Fatalf(
					"message.completed text = %q, must not treat COMPLETE-bearing partial stdout as terminal success on timeout",
					text.Text,
				)
			}
		}
	}

	if inference.Response != nil && strings.Contains(*inference.Response, "COMPLETE") {
		t.Fatalf(
			"timeout inference response = %q, must not treat COMPLETE-bearing stdout as success",
			*inference.Response,
		)
	}

	dispatches := support.ObserveDispatchEvents(t, replay.FactoryEvents)
	if len(dispatches) == 0 {
		t.Fatal("missing public dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response != nil && response.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("terminal dispatch outcome = %#v, must not invent ACCEPTED work outcome on timeout", response)
	}
}

func projectCodexFailureProviderSessionGolden(
	loaded support.ProviderSessionCase,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
	session := map[string]any{
		"provider":      loaded.Manifest.Provider,
		"fidelityClass": loaded.Manifest.FidelityClass,
		"status":        "failed",
	}
	if inference.Outcome == factoryapi.InferenceOutcomeSucceeded {
		session["status"] = "completed"
	}

	sessionID := ""
	if inference.ProviderSession != nil && inference.ProviderSession.Id != nil {
		sessionID = strings.TrimSpace(*inference.ProviderSession.Id)
	}
	if sessionID == "" && inference.Response != nil {
		sessionID = codexThreadIDFromTranscript(*inference.Response)
	}
	session["providerSessionId"] = sessionID
	return session
}

func projectCodexFailureResponseEventGoldens(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) []json.RawMessage {
	t.Helper()

	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		record := projectCodexFailureResponseEventGolden(event)
		if len(record) == 0 {
			continue
		}
		records = append(records, record)
	}
	return records
}

func projectCodexFailureResponseEventGolden(event factoryapi.FactoryResponseEvent) json.RawMessage {
	switch event.Kind {
	case factoryapi.FactoryResponseEventKindError:
		if event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
			return nil
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			return nil
		}
		record := map[string]any{
			"type":    "error.failed",
			"eventId": event.EventId,
			"code":    payload.Code,
			"message": payload.Message,
		}
		if event.FactorySessionId != "" {
			record["factorySessionId"] = event.FactorySessionId
		}
		if event.RunId != "" {
			record["runId"] = event.RunId
		}
		if event.ItemId != nil && strings.TrimSpace(*event.ItemId) != "" {
			record["itemId"] = strings.TrimSpace(*event.ItemId)
		}
		if payload.Retryable != nil {
			record["retryable"] = *payload.Retryable
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return nil
		}
		return encoded
	case factoryapi.FactoryResponseEventKindRun:
		payload, err := event.Payload.AsFactoryResponseEventRunPayload()
		if err == nil && payload.Status != nil && strings.TrimSpace(*payload.Status) != "" {
			record := map[string]any{
				"type":   strings.ToLower(string(event.Kind) + "." + strings.ToLower(string(event.Phase))),
				"status": strings.ToLower(strings.TrimSpace(*payload.Status)),
			}
			if nativeType := strings.TrimSpace(event.Provenance.NativeEventType); nativeType != "" {
				record["type"] = strings.ToLower(nativeType)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				return nil
			}
			return encoded
		}
	}
	return projectCodexResponseEventGolden(event)
}

func projectCodexFailureInvocationResultGolden(
	t *testing.T,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
	t.Helper()

	result := map[string]any{
		"ok": false,
	}
	if inference.FailureDetail != nil {
		result["failureReason"] = inference.FailureDetail.Reason
		result["message"] = inference.FailureDetail.Message
	}
	return result
}
