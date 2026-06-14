package factorysessionexecution

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReplaySessionProjection_TerminalSessionBracket(t *testing.T) {
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-001"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource: ResolvedSource{
				SourceRef:  "workflow/simple-final",
				SourceHash: "sha256:fixture",
			},
			ResultSummary: &ResultSummary{
				ResultStatus: string(ResultStatusFinal),
				Summary:      "Completed simple workflow.",
			},
			Lifecycle: &LifecycleTimestamps{
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
			},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
		},
	)

	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", session.Status)
	}
	if session.SourceHash != "sha256:fixture" {
		t.Fatalf("sourceHash = %q", session.SourceHash)
	}
	if session.Policy.EffectiveHash != "sha256:policy" {
		t.Fatalf("policyHash = %q", session.Policy.EffectiveHash)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
	if session.Links.Session == "" || session.Links.Events == "" {
		t.Fatal("expected inspection links")
	}
	if result.ResultStatus != ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
}

func TestReplaySessionProjection_IdempotentOnDuplicateSequence(t *testing.T) {
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-002"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)

	firstSession, firstResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}
	if firstSession.SessionID != secondSession.SessionID ||
		firstSession.Status != secondSession.Status ||
		firstSession.SourceHash != secondSession.SourceHash ||
		firstSession.Policy.EffectiveHash != secondSession.Policy.EffectiveHash ||
		firstSession.Links.Session != secondSession.Links.Session {
		t.Fatalf("session projection changed on replay: %#v vs %#v", firstSession, secondSession)
	}
	if firstResult.ResultStatus != secondResult.ResultStatus ||
		firstResult.SessionStatus != secondResult.SessionStatus {
		t.Fatalf("result projection changed on replay: %#v vs %#v", firstResult, secondResult)
	}
}

func TestReplaySessionProjection_ReplacesArtifactStubsWithoutDuplication(t *testing.T) {
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-003"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
			ArtifactIDs:  []string{"art-1", "art-2"},
		},
	)

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if len(session.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs = %d, want 2", len(session.ArtifactRefs))
	}

	// Replaying the same events again must not duplicate artifact stubs.
	sessionAgain, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection again: %v", err)
	}
	if len(sessionAgain.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs after second replay = %d, want 2", len(sessionAgain.ArtifactRefs))
	}
}

func TestReplaySessionProjection_PreservesSyncTimeoutAvailability(t *testing.T) {
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-timeout-availability"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
			Availability: &ResultAvailabilityDetail{
				Reason:    "SYNC_WAIT_TIMED_OUT",
				Message:   "Sync wait ended before a terminal result was available.",
				Retryable: true,
			},
		},
	)

	_, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestReplaySessionProjection_IgnoresUnknownEventTypes(t *testing.T) {
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-004"
	base := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)
	events := append(append([]json.RawMessage(nil), base...), json.RawMessage(`{"type":"DISPATCH_QUEUED","id":"dispatch-queued/1","context":{"sequence":99},"payload":{}}`))

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, sessionID)
	}
}
