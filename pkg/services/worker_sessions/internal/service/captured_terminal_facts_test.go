package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type terminalSummaryReader struct {
	historyCatalogFake
	item recordings.WorkerCapturedCatalogItem
}

func (f *terminalSummaryReader) LookupWorkerSessionSummary(context.Context, string) (recordings.WorkerCapturedSummary, error) {
	return recordings.WorkerCapturedSummary{Capture: f.item}, nil
}

func completedSummaryFixture(t *testing.T, failed bool) recordings.WorkerCapturedCatalogItem {
	t.Helper()
	item := historyCapture(t, "worker", "factory", "attempt", true)
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	opening, _ := json.Marshal(workers.SessionPayload{WorkerSessionID: "worker", FactorySessionID: "factory", AttemptID: "attempt", WorkIDs: []string{"work"}, StartedAt: &start, Model: "model", ReasoningEffort: "high"})
	item.Opening.Payload, _ = json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: opening})
	item.Catalog.RecordingID, item.Catalog.CommittedPosition = "recording", 3
	item.Terminal.Position, item.Terminal.Phase = 3, workers.PhaseCompleted
	payload := map[string]any{"status": "COMPLETED", "continuation": map[string]string{"provider": "codex", "kind": "session_id", "id": "opaque"}}
	if failed {
		item.Terminal.Status, item.Terminal.Phase = "FAILED", workers.PhaseFailed
		payload["status"], payload["failureCause"], payload["failureDetail"] = "FAILED", "WORKERS_EXECUTION_FAILURE", "expected output artifact was not produced"
	}
	raw, _ := json.Marshal(payload)
	terminal, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: item.Terminal.Phase, DispatchID: "attempt", Payload: raw})
	item.MetadataRecords = []events.Record{
		{ID: events.RecordID{Position: 2}, Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","dispatchId":"attempt","payload":{"inputTokens":0,"totalTokens":12}}`)},
		{ID: events.RecordID{Position: 3}, Payload: terminal},
	}
	item.CapturedAt = map[string]time.Time{"3": start.Add(time.Second)}
	return item
}

func TestCapturedCompletedSummarySelectedCatalogAndLiveAgree(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "failed"}[failed], func(t *testing.T) {
			t.Parallel()
			item := completedSummaryFixture(t, failed)
			fake := &terminalSummaryReader{item: item, historyCatalogFake: historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{item}}}
			logs := &LogReader{reader: fake}
			got, err := logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "factory"})
			if err != nil {
				t.Fatalf("lost completed facts: %+v %v", got, err)
			}
			assertCompletedSummaryFacts(t, got, failed)
			rows, err := logs.archivedHistory(t.Context(), workersessions.ListWorkerSessionObservationsRequest{}, nil)
			if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], got) {
				t.Fatalf("catalog differs: %+v %v", rows, err)
			}
			live := (&registry{logs: logs, publications: map[string]*publication{scopedWorkerAddress("worker", "factory"): {recordingID: "recording"}}}).withCapturedTerminalObservation(t.Context(), workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "factory", AttemptID: "attempt", State: got.State, EndedAt: got.StartedAt})
			if !reflect.DeepEqual(live.EndedAt, got.EndedAt) || !reflect.DeepEqual(live.Failure, got.Failure) || live.ProviderSession != got.ProviderSession || live.Transcript != got.Transcript {
				t.Fatalf("live differs: %+v", live)
			}
			if _, err := logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "foreign"}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
				t.Fatalf("foreign scope: %v", err)
			}
			got.WorkIDs[0], *got.Model, *got.TokenUsage.TotalTokens = "mutated", "mutated", 999
			again, err := logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
			if err != nil || *again.Model != "model" || *again.TokenUsage.TotalTokens != 12 || again.WorkIDs[0] != "work" {
				t.Fatalf("aliased facts: %+v %v", again, err)
			}
		})
	}
}

func assertCompletedSummaryFacts(t *testing.T, got workersessions.Observation, failed bool) {
	t.Helper()
	if !got.ProviderSessionAvailable || got.ProviderSession.ID != "opaque" || got.Transcript != workersessions.TranscriptAvailabilityAvailable || got.Duration == nil || *got.Duration != time.Second || got.TokenUsage == nil || *got.TokenUsage.TotalTokens != 12 {
		t.Fatalf("lost completed facts: %+v", got)
	}
	if failed {
		if got.State != workersessions.StateFailed || got.Failure == nil || got.Failure.Detail != "expected output artifact was not produced" || got.TerminalCause == nil || *got.TerminalCause != "FAILED" {
			t.Fatalf("lost execution failure: %+v", got)
		}
	}
}

func TestCapturedTerminalFactsRejectForeignAndMalformedMetadata(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"foreign-worker", "foreign-attempt", "foreign-scope", "invalid-ref", "invalid-failure", "corrupt", "uncommitted", "degraded", "legacy"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := completedSummaryFixture(t, true)
			var draft workers.Draft
			_ = json.Unmarshal(item.MetadataRecords[1].Payload, &draft)
			var payload map[string]any
			_ = json.Unmarshal(draft.Payload, &payload)
			mutateCompletedSummaryFixture(name, &draft, payload, &item)
			draft.Payload, _ = json.Marshal(payload)
			item.MetadataRecords[1].Payload, _ = json.Marshal(draft)
			if name == "corrupt" {
				item.MetadataRecords[1].Payload = []byte(`{`)
			}
			got, err := capturedHistoryIdentity(item, nil)
			if name == "uncommitted" || name == "degraded" || name == "legacy" {
				if err != nil || got.Transcript != workersessions.TranscriptAvailabilityUnavailable {
					t.Fatalf("invented availability: %+v %v", got, err)
				}
				if name == "legacy" && (got.ProviderSessionAvailable || got.Failure != nil || got.EndedAt != nil) {
					t.Fatalf("invented legacy facts: %+v", got)
				}
			} else if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
				t.Fatalf("unsafe facts: %+v %v", got, err)
			}
		})
	}
}

func mutateCompletedSummaryFixture(name string, draft *workers.Draft, payload map[string]any, item *recordings.WorkerCapturedCatalogItem) {
	switch name {
	case "foreign-worker":
		payload["workerSessionId"] = "sibling"
	case "foreign-attempt":
		draft.DispatchID = "sibling"
	case "foreign-scope":
		payload["factorySessionId"] = "sibling"
	case "invalid-ref":
		payload["continuation"] = map[string]string{"provider": "codex"}
	case "invalid-failure":
		payload["failureCause"] = "UNKNOWN"
	case "uncommitted":
		item.Catalog.CommittedPosition = 2
	case "degraded":
		item.Health = recordings.WorkerRecordingStatusDegraded
	case "legacy":
		delete(payload, "continuation")
		delete(payload, "failureCause")
		delete(payload, "failureDetail")
		item.CapturedAt = nil
	}
}

func TestCapturedTerminalEnrichmentPreservesLiveFailureClassification(t *testing.T) {
	t.Parallel()
	item := completedSummaryFixture(t, true)
	reader := &terminalSummaryReader{item: item}
	r := &registry{logs: &LogReader{reader: reader}, publications: map[string]*publication{scopedWorkerAddress("worker", "factory"): {recordingID: "recording"}}}
	failure := &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "live detail", ProviderFailureKind: providers.ExecuteFailureKindTimeout, ProviderContinuationFailureKind: providers.ContinuationFailureKindStale, ProviderContinuationOutcome: providers.ContinuationOutcomeUnsupported, AgentRunFailureClass: "agent_run_provider_failure"}
	observation := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "factory", AttemptID: "attempt", State: workersessions.StateFailed, Failure: failure}
	got := r.withCapturedTerminalObservation(t.Context(), observation)
	want := *failure
	want.Detail = "expected output artifact was not produced"
	if !reflect.DeepEqual(got.Failure, &want) || failure.Detail != "live detail" {
		t.Fatalf("lost or mutated normalized live failure: %+v", got.Failure)
	}
	archived, err := r.logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "factory"})
	if err != nil || archived.Failure.ProviderFailureKind != "" || archived.Failure.ProviderContinuationFailureKind != "" || archived.Failure.ProviderContinuationOutcome != "" {
		t.Fatalf("inferred unrecorded classification: %+v %v", archived.Failure, err)
	}
}

func TestCapturedTerminalEnrichmentPreservesUnrecordedAndControlledTiming(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	end, duration := start.Add(time.Second), time.Second
	for _, state := range []workersessions.State{workersessions.StateCompleted, workersessions.StateFailed, workersessions.StateCanceled, workersessions.StateTerminated} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			observation := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "factory", State: state, StartedAt: &start, EndedAt: &end, Duration: &duration, DurationBasis: workersessions.DurationBasisRecordedTimestamps}
			for _, recordingID := range []string{"", "recording"} {
				if recordingID != "" && (state == workersessions.StateCompleted || state == workersessions.StateFailed) {
					continue
				}
				r := &registry{publications: map[string]*publication{scopedWorkerAddress("worker", "factory"): {recordingID: recordingID}}}
				if got := r.withCapturedTerminalObservation(t.Context(), observation); !reflect.DeepEqual(got, observation) {
					t.Fatalf("lost lifecycle timing: %+v", got)
				}
			}
		})
	}
}
