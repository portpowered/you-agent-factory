package service

import (
	"encoding/json"
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
