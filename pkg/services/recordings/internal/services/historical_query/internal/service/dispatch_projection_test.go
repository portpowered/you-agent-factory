package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProjectHistoricalDispatchesCarriesPetriResponseUsage(t *testing.T) {
	t.Parallel()

	duration := int64(1500)
	inputTokens := int64(12)
	outputTokens := int64(8)
	totalTokens := int64(20)
	dispatches, err := projectHistoricalDispatches(
		recordings.HistoricalRecordingIdentity{RecordingID: "recording-usage-001"},
		[]recordings.CanonicalEvent{historicalDispatchEvent(t, "dispatch-usage-present", factorydefinitions.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			Outcome:      workerexecution.OutcomeAccepted,
			TransitionID: "build",
			Usage: &workerexecution.DispatchUsageEventPayload{
				DurationMillis: &duration,
				InputTokens:    &inputTokens,
				OutputTokens:   &outputTokens,
				TotalTokens:    &totalTokens,
			},
		})},
	)
	if err != nil {
		t.Fatalf("projectHistoricalDispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Usage == nil {
		t.Fatalf("dispatches = %#v, want one dispatch with usage", dispatches)
	}
	usage := dispatches[0].Usage
	if usage.DurationMillis == nil || *usage.DurationMillis != duration ||
		usage.InputTokens == nil || *usage.InputTokens != inputTokens ||
		usage.OutputTokens == nil || *usage.OutputTokens != outputTokens ||
		usage.TotalTokens == nil || *usage.TotalTokens != totalTokens {
		t.Fatalf("dispatch usage = %#v, want duration and token facts", usage)
	}
	if usage.CostUSD != nil {
		t.Fatalf("dispatch cost = %#v, want unset", usage.CostUSD)
	}
}

func TestProjectHistoricalDispatchesPreservesCanceledResponse(t *testing.T) {
	t.Parallel()
	dispatches, err := projectHistoricalDispatches(
		recordings.HistoricalRecordingIdentity{RecordingID: "recording-canceled"},
		[]recordings.CanonicalEvent{historicalDispatchEvent(t, "dispatch-canceled", factorydefinitions.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			Outcome: workerexecution.OutcomeCanceled, TransitionID: "process",
		})},
	)
	if err != nil || len(dispatches) != 1 || dispatches[0].Status != recordings.FactoryDispatchStatusInterrupted {
		t.Fatalf("canceled dispatch projection = %+v, %v; want interrupted", dispatches, err)
	}
}

func TestProjectHistoricalDispatchesRetainsDurationWhenPetriTokensAreAbsent(t *testing.T) {
	t.Parallel()

	duration := int64(2000)
	dispatches, err := projectHistoricalDispatches(
		recordings.HistoricalRecordingIdentity{RecordingID: "recording-usage-002"},
		[]recordings.CanonicalEvent{historicalDispatchEvent(t, "dispatch-usage-absent", factorydefinitions.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			Outcome:      workerexecution.OutcomeAccepted,
			TransitionID: "build",
			Usage:        &workerexecution.DispatchUsageEventPayload{DurationMillis: &duration},
		})},
	)
	if err != nil {
		t.Fatalf("projectHistoricalDispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Usage == nil || dispatches[0].Usage.DurationMillis == nil || *dispatches[0].Usage.DurationMillis != duration {
		t.Fatalf("dispatches = %#v, want duration-only usage", dispatches)
	}
	usage := dispatches[0].Usage
	if usage.InputTokens != nil || usage.OutputTokens != nil || usage.TotalTokens != nil || usage.CostUSD != nil {
		t.Fatalf("duration-only usage = %#v, want token and cost facts omitted", usage)
	}
}

func TestProjectHistoricalDispatchesDoesNotCopyJavaScriptReconciliationUsage(t *testing.T) {
	t.Parallel()

	inputTokens := int64(12)
	outputTokens := int64(8)
	totalTokens := int64(20)
	dispatchID := "dispatch-javascript-usage"
	queued := historicalDispatchEvent(t, dispatchID, factorydefinitions.FactoryEventTypeDispatchQueued, factorydefinitions.DispatchQueuedEventPayload{
		DispatchKind: factorydefinitions.FactoryDispatchKindJavaScriptScript,
	})
	reconciled := historicalDispatchEvent(t, dispatchID, factorydefinitions.FactoryEventTypeDispatchReconciled, factorydefinitions.DispatchReconciledEventPayload{
		ReconciledStatus: factorydefinitions.FactoryDispatchStatusCompleted,
		Usage: &factorydefinitions.FactoryDispatchUsage{
			InputTokens: &inputTokens, OutputTokens: &outputTokens, TotalTokens: &totalTokens,
		},
	})
	dispatches, err := projectHistoricalDispatches(
		recordings.HistoricalRecordingIdentity{RecordingID: "recording-usage-003"},
		[]recordings.CanonicalEvent{queued, reconciled},
	)
	if err != nil {
		t.Fatalf("projectHistoricalDispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].DispatchKind != recordings.FactoryDispatchKindJavaScriptScript {
		t.Fatalf("dispatches = %#v, want one JavaScript dispatch", dispatches)
	}
	if dispatches[0].Usage != nil {
		t.Fatalf("JavaScript usage = %#v, want existing reconciliation output unchanged", dispatches[0].Usage)
	}
}

func historicalDispatchEvent(
	t *testing.T,
	dispatchID string,
	eventType factorydefinitions.FactoryEventType,
	payload any,
) recordings.CanonicalEvent {
	t.Helper()

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", eventType, err)
	}
	eventTime := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	return canonical.CanonicalEventFromFactory(factorydefinitions.FactoryEvent{
		Type: eventType,
		Id:   "factory-event/" + strings.ToLower(string(eventType)) + "/" + dispatchID,
		Context: factorydefinitions.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  eventTime,
			Sequence:   1,
			Tick:       1,
		},
		Payload: encoded,
	}, "historical-usage-test")
}

func TestHistoricalDispatchesIgnoreUnrelatedEventsWithoutLosingValidation(t *testing.T) {
	t.Parallel()
	identity := recordings.HistoricalRecordingIdentity{RecordingID: "mixed-history"}
	request := historicalDispatchEvent(t, "dispatch", factorydefinitions.FactoryEventTypeDispatchRequest,
		factorydefinitions.DispatchRequestEventPayload{TransitionID: "process"})
	association := historicalDispatchEvent(t, "dispatch", factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
		factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker"})
	response := historicalDispatchEvent(t, "dispatch", factorydefinitions.FactoryEventTypeDispatchResponse,
		workerexecution.DispatchResponseEventPayload{Outcome: workerexecution.OutcomeAccepted, TransitionID: "process"})
	unrelated := historicalDispatchEvent(t, "dispatch", factorydefinitions.FactoryEventTypeWorkRequest,
		map[string]string{"name": "Named Work", "payload": strings.Repeat("x", 1024)})
	want, err := projectHistoricalDispatches(identity, []recordings.CanonicalEvent{request, association, response})
	if err != nil {
		t.Fatal(err)
	}
	got, err := projectHistoricalDispatches(identity, []recordings.CanonicalEvent{unrelated, request, unrelated, association, unrelated, response, unrelated})
	if err != nil || !reflect.DeepEqual(got, want) || len(got) != 1 || got[0].Association.WorkerSessionID != "worker" {
		t.Fatalf("mixed-history dispatch facts = %+v, %v; want %+v", got, err, want)
	}
	// Only the dispatch reducer skips unrelated events. Canonical decoding still
	// owns validation of every event; malformed relevant facts remain errors.
	association.Payload = `{"workerSessionId":""}`
	_, err = projectHistoricalDispatches(identity, []recordings.CanonicalEvent{unrelated, request, association, response})
	var diagnostic *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &diagnostic) || diagnostic.Kind != recordings.HistoricalRecordingQueryErrorCorruptHistory || diagnostic.EventID != association.ID {
		t.Fatalf("malformed association = %v; want corrupt history at %s", err, association.ID)
	}
}

// This bounded component benchmark measures reducer allocation/CPU, not the
// customer GET latency bound or operator-profile equivalence.
func BenchmarkHistoricalDispatchesSparseEvents(b *testing.B) {
	identity := recordings.HistoricalRecordingIdentity{RecordingID: "sparse-history"}
	noise := recordings.CanonicalEvent{
		Kind:          recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:       `{"payload":"` + strings.Repeat("x", 4096) + `"}`,
		SourceContext: `{"sequence":1,"tick":1}`,
	}
	events := make([]recordings.CanonicalEvent, 64)
	for index := range events {
		events[index] = noise
	}
	events[32] = recordings.CanonicalEvent{
		Kind:          recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchResponse),
		Payload:       `{"outcome":"ACCEPTED","transitionId":"process"}`,
		SourceContext: `{"dispatchId":"dispatch","sequence":32,"tick":1}`,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, err := projectHistoricalDispatches(identity, events)
		if err != nil || len(result) != 1 || result[0].Status != recordings.FactoryDispatchStatusCompleted {
			b.Fatalf("dispatch = %+v, %v", result, err)
		}
	}
}
