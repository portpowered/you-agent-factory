package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCapturedSelectedAndCatalogTimingRequireCommittedTerminalStamp(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"recorded", "zero-duration", "legacy", "no-start", "unfinished", "uncaptured-terminal", "clock-reversal"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			end := start.Add(2 * time.Second)
			item := historyCapture(t, "worker", "", "attempt", true)
			item.Catalog.RecordingID, item.Catalog.RecordingGenerationID = "recording", "generation"
			item.Catalog.CommittedPosition = 2
			item.Terminal.Position = 2
			opening := workers.SessionPayload{WorkerSessionID: "worker", AttemptID: "attempt", StartedAt: &start}
			switch name {
			case "zero-duration":
				end = start
			case "no-start":
				opening.StartedAt = nil
			case "unfinished":
				item.Terminal = nil
			case "uncaptured-terminal":
				item.Catalog.CommittedPosition = 1
			case "clock-reversal":
				end = start.Add(-time.Second)
			}
			payload, _ := json.Marshal(opening)
			item.Opening.Payload, _ = json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: payload})
			if name != "legacy" {
				item.CapturedAt = map[string]time.Time{"2": end}
			}
			reader := &capturedMetadataReader{
				capturedActivityFake: &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{Catalog: item.Catalog, Opening: item.Opening, Terminal: item.Terminal, OwnerLost: item.OwnerLost, Health: item.Health}},
				replayCaptureReader: replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{
					{WorkerSessionID: "sibling", CapturedAt: map[string]time.Time{"2": start.Add(time.Hour)}},
					{WorkerSessionID: "worker", RecordingGenerationID: "generation", CapturedAt: item.CapturedAt},
				}}},
			}
			selected, err := (&LogReader{reader: reader}).GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
			if err != nil {
				t.Fatal(err)
			}
			listed, err := capturedHistoryIdentity(item, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, observation := range []workersessions.Observation{selected, *listed} {
				assertCapturedTiming(t, observation, start, end, name == "recorded" || name == "zero-duration")
			}
		})
	}
}

func assertCapturedTiming(t *testing.T, observation workersessions.Observation, start, end time.Time, available bool) {
	t.Helper()
	if available {
		if observation.EndedAt == nil || !observation.EndedAt.Equal(end) || observation.Duration == nil || *observation.Duration != end.Sub(start) || observation.DurationBasis != workersessions.DurationBasisRecordedTimestamps {
			t.Fatalf("recorded timing lost: %+v", observation)
		}
	} else if observation.EndedAt != nil || observation.Duration != nil || observation.DurationBasis != workersessions.DurationBasisUnavailable {
		t.Fatalf("invented timing: %+v", observation)
	}
}

func TestCapturedUsageModelPreservesZeroAndAbsentCounters(t *testing.T) {
	t.Parallel()
	observation := workersessions.Observation{AttemptID: "attempt"}
	applyCapturedUsageFacts(&observation, workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, DispatchID: "attempt", Payload: []byte(`{"inputTokens":0,"model":"usage-model"}`)})
	applyCapturedUsageFacts(&observation, workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: []byte(`{"model":"later-model"}`)})
	applyCapturedUsageFacts(&observation, workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, DispatchID: "sibling", Payload: []byte(`{"inputTokens":999,"model":"foreign"}`)})
	if observation.Model == nil || *observation.Model != "later-model" || observation.TokenUsage == nil || observation.TokenUsage.InputTokens == nil || *observation.TokenUsage.InputTokens != 0 || observation.TokenUsage.TotalTokens != nil {
		t.Fatalf("model update erased usage or admitted sibling facts: %+v", observation)
	}
}

func TestCapturedUsageSummaryNeverReadsProviderFiles(t *testing.T) {
	t.Parallel()
	projector := &trackingObservationProjector{}
	r := newObservationRegistry(projector, nil)
	r.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	r.observations["worker-1"] = observationMetadata()
	liveTokens := 999
	r.observations["worker-1"].tokenUsage = &workersessions.TokenUsage{TotalTokens: &liveTokens}
	r.publications["worker-1"] = &publication{recordingID: "recording"}
	r.recording = replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: "recording",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "sibling", Records: []events.Record{{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"totalTokens":888}}`)}}},
			{WorkerSessionID: "worker-1", Records: []events.Record{
				{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"totalTokens":12}}`)},
				{Payload: []byte(`{"kind":"USAGE","phase":"COMPLETED","payload":{"totalTokens":777}}`)},
			}},
		},
	}}
	req := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}
	got, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || got.TokenUsage == nil || got.TokenUsage.TotalTokens == nil || *got.TokenUsage.TotalTokens != 12 || got.TokenUsage.InputTokens == nil || *got.TokenUsage.InputTokens != 0 {
		t.Fatalf("summary lost committed usage or explicit zero: %+v, %v", got.TokenUsage, err)
	}
	*got.TokenUsage.TotalTokens = 321
	again, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || *again.TokenUsage.TotalTokens != 12 || projector.calls != 0 {
		t.Fatalf("summary aliases usage or reads native provider: %+v calls=%d err=%v", again.TokenUsage, projector.calls, err)
	}
	r.recording = replayCaptureReader{}
	unknown, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || unknown.WorkerSessionID != "worker-1" || unknown.TokenUsage != nil || projector.calls != 0 {
		t.Fatalf("missing capture invented usage or hid identity: %+v calls=%d err=%v", unknown, projector.calls, err)
	}
}

func TestCapturedSessionFactsKeepIdentityAndPartialUpdates(t *testing.T) {
	t.Parallel()
	for _, identity := range []string{"selected", "sibling", "wrong-attempt", "wrong-scope", "corrupt"} {
		t.Run(identity, func(t *testing.T) {
			t.Parallel()
			observation := workersessions.Observation{WorkerSessionID: "selected", AttemptID: "attempt", FactorySessionID: "factory"}
			payload := workers.SessionPayload{WorkerSessionID: "selected", AttemptID: "attempt", FactorySessionID: "factory", Model: "recorded-model", ReasoningEffort: "high"}
			switch identity {
			case "sibling":
				payload.WorkerSessionID = "sibling"
			case "wrong-attempt":
				payload.AttemptID = "other"
			case "wrong-scope":
				payload.FactorySessionID = "other"
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if identity == "corrupt" {
				raw = []byte(`{`)
			}
			applyCapturedSessionFacts(&observation, workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseUpdated, Payload: raw, Provenance: workers.Provenance{Provider: "codex"}})
			if identity != "selected" {
				if observation.Provider != "" || observation.Model != nil || observation.ReasoningEffort != nil {
					t.Fatalf("foreign/corrupt facts enriched summary: %+v", observation)
				}
				return
			}
			applyCapturedSessionFacts(&observation, workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseUpdated, Payload: []byte(`{"workerSessionId":"selected","status":"RUNNING"}`)})
			assertCapturedSessionFacts(t, observation)
		})
	}
}

func assertCapturedSessionFacts(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.Provider != "codex" || observation.Model == nil || *observation.Model != "recorded-model" || observation.ReasoningEffort == nil || *observation.ReasoningEffort != "high" {
		t.Fatalf("partial update erased facts: %+v", observation)
	}
}

type capturedMetadataReader struct {
	*capturedActivityFake
	replayCaptureReader
}

func TestCapturedSelectedAndCatalogSummaryPreserveRecordedFacts(t *testing.T) {
	t.Parallel()
	item := historyCapture(t, "worker", "", "attempt", true)
	item.Catalog.RecordingID = "recording"
	item.Catalog.CommittedPosition = 2
	item.SuccessorWorkerSessionID = "next"
	item.MetadataRecords = []events.Record{{
		ID:      events.RecordID{Position: 2},
		Payload: []byte(`{"kind":"SESSION","phase":"UPDATED","provenance":{"provider":"codex"},"payload":{"workerSessionId":"worker","attemptId":"attempt","model":"recorded-model","reasoningEffort":"high","lineage":{"predecessorWorkerSessionId":"prior"}}}`),
	}}
	reader := &capturedMetadataReader{
		capturedActivityFake: &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{Catalog: item.Catalog, Opening: item.Opening, Terminal: item.Terminal, Health: item.Health, SuccessorWorkerSessionID: item.SuccessorWorkerSessionID}},
		replayCaptureReader: replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "worker", Records: item.MetadataRecords},
			{WorkerSessionID: "sibling", Records: []events.Record{{Payload: []byte(`{"kind":"SESSION","phase":"UPDATED","provenance":{"provider":"foreign"},"payload":{"workerSessionId":"worker"}}`)}}},
		}}},
	}
	got, err := (&LogReader{reader: reader}).GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedSessionFacts(t, got)
	if got.PredecessorWorkerSessionID != "prior" || got.SuccessorWorkerSessionID != "next" {
		t.Fatalf("recorded lineage: %+v", got)
	}
	listed, err := capturedHistoryIdentity(item, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedSessionFacts(t, *listed)
	if listed.PredecessorWorkerSessionID != got.PredecessorWorkerSessionID || listed.SuccessorWorkerSessionID != got.SuccessorWorkerSessionID {
		t.Fatalf("selected/catalog lineage differs: %+v %+v", listed, got)
	}
}

func TestObservationProviderBindingBeforeAssociationAndFailedPublication(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "failed"}[failed], func(t *testing.T) {
			t.Parallel()
			r := providerBindingRegistry(t, nil)
			r.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
			unassociated := r.sessions["worker-1"]
			unassociated.ProviderSessionAssociation = nil
			r.sessions["worker-1"] = unassociated
			r.observations["worker-1"] = observationMetadata()
			if failed {
				r.events = staticProviderBindingAppender{err: errors.New("controlled failure")}
			}
			_, err := r.EnsureProviderBinding(t.Context(), workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "codex"})
			if (err != nil) != failed {
				t.Fatalf("binding: %v", err)
			}
			got, err := r.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"})
			want := "codex"
			if failed {
				want = ""
			}
			if err != nil || got.Provider != want || got.ProviderSessionAvailable {
				t.Fatalf("bound provider without association: %+v %v", got, err)
			}
		})
	}
}
