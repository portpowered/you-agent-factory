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

func TestCapturedTerminalEnrichmentPreservesLiveTimingUntilCaptureComplete(t *testing.T) {
	t.Parallel()
	for _, state := range []workersessions.State{workersessions.StateCompleted, workersessions.StateFailed} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			for _, name := range []string{"pending-terminal", "missing-capture-clock", "foreign-attempt"} {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					item := completedSummaryFixture(t, state == workersessions.StateFailed)
					switch name {
					case "pending-terminal":
						item.Terminal = nil
					case "missing-capture-clock":
						item.CapturedAt = nil
					case "foreign-attempt":
						observationAttempt := workers.SessionPayload{WorkerSessionID: "worker", FactorySessionID: "factory", AttemptID: "foreign"}
						payload, _ := json.Marshal(observationAttempt)
						item.Opening.Payload, _ = json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: payload})
					}
					start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
					end, duration := start.Add(2*time.Second), 2*time.Second
					observation := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "factory", AttemptID: "attempt", State: state, StartedAt: &start, EndedAt: &end, Duration: &duration, DurationBasis: workersessions.DurationBasisRecordedTimestamps}
					r := &registry{logs: &LogReader{reader: &terminalSummaryReader{item: item}}, publications: map[string]*publication{scopedWorkerAddress("worker", "factory"): {recordingID: "recording"}}}
					got := r.withCapturedTerminalObservation(t.Context(), observation)
					if !reflect.DeepEqual(got.StartedAt, observation.StartedAt) || !reflect.DeepEqual(got.EndedAt, observation.EndedAt) || !reflect.DeepEqual(got.Duration, observation.Duration) || got.DurationBasis != observation.DurationBasis {
						t.Fatalf("lost owned terminal timing before complete capture: %+v", got)
					}
				})
			}
		})
	}
}

func TestCapturedRequesterMetadataIsDetachedAndValidated(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"requester":{"kind":"WORKER_SESSION","workerSessionId":"lead","workId":"project"},"correlation":{"workId":"lane","factorySessionId":"factory"},"labels":["tag:project=example"]}`,
		`{"requester":null}`,
		``,
	} {
		item := completedSummaryFixture(t, false)
		var draft workers.Draft
		var opening workers.SessionPayload
		_ = json.Unmarshal(item.Opening.Payload, &draft)
		_ = json.Unmarshal(draft.Payload, &opening)
		opening.SessionMetadata = json.RawMessage(raw)
		draft.Payload, _ = json.Marshal(opening)
		item.Opening.Payload, _ = json.Marshal(draft)
		fake := &terminalSummaryReader{item: item, historyCatalogFake: historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{item}}}
		logs := &LogReader{reader: fake}
		got, err := logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := logs.archivedHistory(t.Context(), workersessions.ListWorkerSessionObservationsRequest{}, nil)
		if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], got) {
			t.Fatalf("show/list metadata differs: %+v %v", rows, err)
		}
		if raw == "" || raw == `{"requester":null}` {
			if got.Requester != nil || got.Correlation != nil || len(got.Labels) != 0 {
				t.Fatalf("invented metadata: %+v", got)
			}
			continue
		}
		want, err := decodeSessionMetadata(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		actual := &workersessions.SessionMetadata{Requester: got.Requester, Correlation: got.Correlation, Labels: got.Labels}
		if !reflect.DeepEqual(actual, want) {
			t.Fatalf("lost requester facts: %+v", got)
		}
		got.Requester.WorkerSessionID, got.Correlation.WorkID, got.Labels[0] = "mutated", "mutated", "mutated"
		again, err := logs.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
		if err != nil || !reflect.DeepEqual(again, rows[0]) {
			t.Fatalf("read aliased metadata: %+v %v", again, err)
		}
	}
}

func TestCapturedMetadataDecoderRejectsUnknownDuplicateAndIncompleteFacts(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`null`, `{}`, `{"requester":null,"token":"planted-secret"}`,
		`{"requester":null,"requester":null}`,
		`{"requester":{"kind":"WORKER_SESSION","workerSessionId":"lead","token":"secret"}}`,
		`{"requester":{"kind":"OPERATOR","workerSessionId":"lead"}}`,
		`{"requester":null,"correlation":{"workId":"lane","workId":"foreign"}}`,
		`{"requester":{"kind":"WORKER_SESSION","workerSessionId":""}}`,
	} {
		if got, err := decodeSessionMetadata(json.RawMessage(raw)); !errors.Is(err, workersessions.ErrInvalidSessionMetadata) || got != nil {
			t.Fatalf("invalid captured metadata accepted: %s, %+v, %v", raw, got, err)
		}
	}
}

func TestArchivedContinuationMetadataMustMatchOpening(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"retained", "legacy", "changed", "invalid"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			item := completedSummaryFixture(t, false)
			page := recordings.WorkerCapturedActivityPage{Opening: item.Opening, Terminal: item.Terminal}
			metadata := json.RawMessage(`{"requester":{"kind":"WORKER_SESSION","workerSessionId":"lead"},"labels":["tag:project=example"]}`)
			if cell == "legacy" {
				metadata = nil
			}
			var draft workers.Draft
			var opening workers.SessionPayload
			_ = json.Unmarshal(page.Opening.Payload, &draft)
			_ = json.Unmarshal(draft.Payload, &opening)
			opening.SessionMetadata = metadata
			draft.Payload, _ = json.Marshal(opening)
			page.Opening.Payload, _ = json.Marshal(draft)
			captured := recordings.WorkerContinuationSource{SessionMetadata: metadata, Terminal: *item.Terminal,
				Reference: providers.SessionRef{Provider: "codex", Kind: "session_id", ID: "opaque"}}
			if cell == "changed" {
				captured.SessionMetadata = json.RawMessage(`{"requester":null}`)
			}
			if cell == "invalid" {
				captured.SessionMetadata = json.RawMessage(`{"requester":null,"token":"planted-secret"}`)
			}
			got, err := archivedContinuationSnapshot(page, recordings.WorkerControlTarget{WorkerSessionID: "worker", ExpectedAttemptID: "attempt"}, captured)
			if cell == "changed" || cell == "invalid" {
				if !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) || got != nil {
					t.Fatalf("unproved requester admitted: %+v %v", got, err)
				}
				return
			}
			want, _ := decodeSessionMetadata(metadata)
			if err != nil || !reflect.DeepEqual(got.snapshot.session.Metadata, want) {
				t.Fatalf("archived requester lost: %+v %v", got, err)
			}
		})
	}
}

func TestRestartRecipeUsesDetachedReservedMetadata(t *testing.T) {
	t.Parallel()
	r, plan, _ := newDurableInterruptFixture(t)
	store := &restartRecipeStore{}
	r.restart = store
	r.observations["worker"] = &observation{direct: true}
	session := r.sessions["worker"]
	session.Metadata = &workersessions.SessionMetadata{
		Requester:   &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "lead", WorkID: "project"},
		Correlation: &workersessions.Correlation{WorkID: "lane", FactorySessionID: "factory"},
		Labels:      []string{"tag:project=example"},
	}
	r.sessions["worker"] = session
	if err := r.saveRestartRecipe(t.Context(), workersessions.InvokeSessionRequest{
		ID: "worker", Execution: plan.execution,
		Metadata: &workersessions.SessionMetadata{Labels: []string{"untrusted-replacement"}},
	}); err != nil {
		t.Fatal(err)
	}
	metadata, err := decodeSessionMetadata(store.metadata)
	if err != nil || !reflect.DeepEqual(metadata, session.Metadata) {
		t.Fatalf("recipe lost reserved facts: %s %v", store.metadata, err)
	}
	session.Metadata.Requester.WorkerSessionID = "mutated"
	session.Metadata.Labels[0] = "mutated"
	if metadata.Requester.WorkerSessionID != "lead" || metadata.Labels[0] != "tag:project=example" {
		t.Fatal("recipe shares reserved metadata")
	}
}
