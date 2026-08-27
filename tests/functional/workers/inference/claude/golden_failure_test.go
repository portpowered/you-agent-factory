package claude

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	claudeGoldenStructuredFailureCase = "structured-failure"
	claudeGoldenTimeoutCase           = "timeout"
)

func assertClaudeGoldenManifest(t *testing.T, loaded support.ProviderSessionCase, wantID string) {
	t.Helper()
	if loaded.Manifest.ID != wantID {
		t.Fatalf("manifest.ID = %q, want %q", loaded.Manifest.ID, wantID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFinalOnly,
		)
	}
}

func assertClaudeGoldenResponseStreamClosesWithoutSuccess(
	t *testing.T,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	if len(responseEvents) == 0 {
		t.Fatal("response stream missing events; want closed terminal stream")
	}
	last := responseEvents[len(responseEvents)-1]
	if last.Phase != factoryapi.FactoryResponseEventPhaseFailed {
		t.Fatalf("terminal response event phase = %q, want FAILED", last.Phase)
	}
	if last.Kind == factoryapi.FactoryResponseEventKindError {
		payload, err := last.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode terminal ERROR response event: %v", err)
		}
		if payload.Message != "Claude request timed out" {
			t.Fatalf("terminal response error message = %q, want Claude request timed out", payload.Message)
		}
	}
	for _, event := range responseEvents {
		if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
			switch event.Kind {
			case factoryapi.FactoryResponseEventKindMessage:
				t.Fatalf("response stream invented successful terminal message: %#v", event)
			case factoryapi.FactoryResponseEventKindRun:
				payload, err := event.Payload.AsFactoryResponseEventRunPayload()
				if err != nil {
					t.Fatalf("decode RUN response event: %v", err)
				}
				if payload.Status != nil && *payload.Status == "completed" {
					t.Fatalf("response stream invented successful run completion: %#v", event)
				}
			}
		}
	}
}

func claudeGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	return claudeGoldenFailedInferenceObservationWithReason(
		t,
		events,
		factoryapi.WorkFailureTypePermanentBadRequest,
	)
}

func claudeGoldenFailedInferenceObservationWithReason(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantReason factoryapi.WorkFailureType,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
	)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode INFERENCE_RESPONSE %q: %v", event.Id, err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		if payload.FailureDetail == nil || payload.FailureDetail.Reason != wantReason {
			continue
		}
		inferencePayload = payload
		foundInference = true
	}
	if !foundInference {
		t.Fatalf("missing failed INFERENCE_RESPONSE with reason %q in factory events", wantReason)
	}
	return inferencePayload
}

func observeClaudeFailedProviderSessionGoldens(
	t *testing.T,
	inferenceResponse factoryapi.InferenceResponseEventPayload,
	responseEvents []factoryapi.FactoryResponseEvent,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSessionRaw, err := marshalProviderSessionGoldenJSON(inferenceResponse.ProviderSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}

	responseEventRecords := make([]json.RawMessage, 0, len(responseEvents))
	for index, event := range responseEvents {
		record, err := marshalProviderSessionGoldenJSON(event)
		if err != nil {
			t.Fatalf("marshal observed response event[%d]: %v", index, err)
		}
		responseEventRecords = append(responseEventRecords, record)
	}

	invocationResult, err := marshalProviderSessionGoldenJSON(claudeFailedInvocationResultGolden(inferenceResponse))
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:  providerSessionRaw,
		ResponseEvents:   responseEventRecords,
		InvocationResult: invocationResult,
	}
}

type claudeFailedInvocationResultGoldenView struct {
	OK            bool   `json:"ok"`
	Content       string `json:"content,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
	Message       string `json:"message,omitempty"`
}

func claudeFailedInvocationResultGolden(
	inferenceResponse factoryapi.InferenceResponseEventPayload,
) claudeFailedInvocationResultGoldenView {
	view := claudeFailedInvocationResultGoldenView{OK: false}
	if inferenceResponse.FailureDetail != nil {
		view.FailureReason = string(inferenceResponse.FailureDetail.Reason)
		view.Message = inferenceResponse.FailureDetail.Message
	}
	if inferenceResponse.Response != nil {
		view.Content = strings.TrimSpace(*inferenceResponse.Response)
	}
	return view
}
