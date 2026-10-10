package runtime

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestCapturedTerminalSummaryUsesExactOriginalScopeAndCaptureTiming(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"captured", "legacy", "wrong-attempt", "different-outcome"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
			end, duration := start.Add(time.Second), time.Second
			captured := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "original", AttemptID: "attempt", State: workersessions.StateFailed,
				StartedAt: &start, EndedAt: &end, Duration: &duration, DurationBasis: workersessions.DurationBasisRecordedTimestamps,
				Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "expected artifact was not produced"}}
			if name == "legacy" {
				captured.EndedAt, captured.Duration = nil, nil
				captured.DurationBasis = workersessions.DurationBasisUnavailable
			}
			if name == "wrong-attempt" {
				captured.AttemptID = "other"
			}
			if name == "different-outcome" {
				captured.State = workersessions.StateCompleted
			}
			peer := &capturedForceService{observation: captured}
			service := &recordedWorkerSessionObservation{Service: peer}
			item := recordings.WorkerCapturedCatalogItem{Catalog: recordings.WorkerSessionCatalogEntry{FactorySessionID: "original", CommittedPosition: 3}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3}}
			got, err := service.withSelectedCapturedTerminal(t.Context(), workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "rebound", AttemptID: "attempt", State: workersessions.StateFailed, EndedAt: &start}, item)
			if err != nil || got.FactorySessionID != "rebound" || got.State != workersessions.StateFailed || peer.request.FactorySessionID != "original" {
				t.Fatalf("physical selection changed canonical identity: %+v %v %+v", got, err, peer.request)
			}
			assertSelectedTerminalSummaryTiming(t, name, got, end, captured.Failure.Detail)
		})
	}
}

func assertSelectedTerminalSummaryTiming(t *testing.T, name string, got workersessions.Observation, end time.Time, detail string) {
	t.Helper()
	if name == "captured" {
		if got.EndedAt == nil || !got.EndedAt.Equal(end) || got.Duration == nil || *got.Duration != time.Second || got.Failure == nil || got.Failure.Detail != detail {
			t.Fatalf("captured terminal facts lost: %+v", got)
		}
	} else if got.EndedAt != nil || got.Duration != nil || got.DurationBasis != workersessions.DurationBasisUnavailable {
		t.Fatalf("invented terminal timing: %+v", got)
	}
}
