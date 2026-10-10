package runtime

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestCapturedTerminalSummaryUsesExactOriginalScopeAndCaptureTiming(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"captured", "legacy", "wrong-attempt", "different-outcome", "owner-loss", "owner-loss-wrong-attempt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
			end, duration := start.Add(time.Second), time.Second
			captured := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "original", AttemptID: "attempt", State: workersessions.StateFailed,
				Provider: "codex", ContinuationHeadWorkerSessionID: "worker",
				StartedAt: &start, EndedAt: &end, Duration: &duration, DurationBasis: workersessions.DurationBasisRecordedTimestamps,
				Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "expected artifact was not produced"}}
			if name == "legacy" || name == "owner-loss" || name == "owner-loss-wrong-attempt" {
				captured.EndedAt, captured.Duration = nil, nil
				captured.DurationBasis = workersessions.DurationBasisUnavailable
			}
			if name == "wrong-attempt" || name == "owner-loss-wrong-attempt" {
				captured.AttemptID = "other"
			}
			if name == "different-outcome" {
				captured.State = workersessions.StateCompleted
				captured.Failure = nil
				cause := "COMPLETED"
				captured.TerminalCause = &cause
			}
			peer := &capturedForceService{observation: captured}
			service := &recordedWorkerSessionObservation{Service: peer}
			item := recordings.WorkerCapturedCatalogItem{Catalog: recordings.WorkerSessionCatalogEntry{FactorySessionID: "original", CommittedPosition: 3}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3}}
			if name == "owner-loss" || name == "owner-loss-wrong-attempt" {
				service.restoredWorkerScopes = map[string]string{"worker": "original"}
				item.Terminal, item.Health = nil, recordings.WorkerRecordingStatusIncomplete
				cause := "OWNER_LOST"
				captured.TerminalCause = &cause
				peer.observation = captured
			}
			got, err := service.withSelectedCapturedTerminal(t.Context(), workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "rebound", AttemptID: "attempt", State: workersessions.StateFailed, EndedAt: &start}, item)
			wantScope, wantState := "original", captured.State
			if name == "wrong-attempt" || name == "owner-loss-wrong-attempt" {
				wantScope, wantState = "rebound", workersessions.StateFailed
			}
			if err != nil || got.FactorySessionID != wantScope || got.State != wantState || peer.request.FactorySessionID != "original" {
				t.Fatalf("physical selection lost exact captured identity/state: %+v %v %+v", got, err, peer.request)
			}
			assertSelectedTerminalSummaryTiming(t, name, got, end)
			if name != "wrong-attempt" && name != "owner-loss-wrong-attempt" && (got.Provider != "codex" || got.ContinuationHeadWorkerSessionID != "worker") {
				t.Fatalf("captured provider/head lost: %+v", got)
			}
			if name == "owner-loss" && (got.TerminalCause == nil || *got.TerminalCause != "OWNER_LOST" || got.Failure.Detail != captured.Failure.Detail) {
				t.Fatalf("captured owner-loss facts lost: %+v", got)
			}
		})
	}
}

func assertSelectedTerminalSummaryTiming(t *testing.T, name string, got workersessions.Observation, end time.Time) {
	t.Helper()
	if name == "captured" || name == "different-outcome" {
		if got.EndedAt == nil || !got.EndedAt.Equal(end) || got.Duration == nil || *got.Duration != time.Second {
			t.Fatalf("captured terminal facts lost: %+v", got)
		}
		assertSelectedTerminalOutcome(t, name, got)
	} else if got.EndedAt != nil || got.Duration != nil || got.DurationBasis != workersessions.DurationBasisUnavailable {
		t.Fatalf("invented terminal timing: %+v", got)
	}
}

func assertSelectedTerminalOutcome(t *testing.T, name string, got workersessions.Observation) {
	t.Helper()
	if name == "captured" && (got.Failure == nil || got.Failure.Detail != "expected artifact was not produced") {
		t.Fatalf("physical failure lost: %+v", got)
	}
	if name == "different-outcome" && (got.Failure != nil || got.TerminalCause == nil || *got.TerminalCause != "COMPLETED") {
		t.Fatalf("Work rejection overwrote physical success: %+v", got)
	}
}
