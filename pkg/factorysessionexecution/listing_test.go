package factorysessionexecution

import (
	"testing"
	"time"

	jssource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestNormalizeListSessionsRequest_DefaultsToLiveAndRejectsUnsupportedScope(t *testing.T) {
	normalized, err := NormalizeListSessionsRequest(ListSessionsRequest{})
	if err != nil {
		t.Fatalf("NormalizeListSessionsRequest: %v", err)
	}
	if normalized.Scope != SessionListScopeLive {
		t.Fatalf("scope = %q, want live", normalized.Scope)
	}

	_, err = NormalizeListSessionsRequest(ListSessionsRequest{Scope: SessionListScope("workspace")})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestApplySessionListScope_LivePersistedAndAllDedup(t *testing.T) {
	startedAt := time.Date(2026, 6, 8, 10, 0, 1, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	base := ListSessionsResult{
		LiveSessions: []LiveSessionSummary{
			{ID: "live-alpha", Project: "alpha"},
			{ID: "dur-sess-petri-success-001", Project: "alpha"},
			{ID: "live-zeta", Project: "zeta"},
		},
		DurableSessions: []DurableSessionListSummary{
			{
				SessionID:        "dur-sess-petri-success-001",
				Status:           LifecycleStatusSucceeded,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       jssource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusFinal)},
				Progress:      &ProgressCounts{TotalDispatches: 1, CompletedDispatches: 1},
				ArtifactCount: 1,
				Lifecycle:     &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusSucceeded),
			},
			{
				SessionID:        "dur-sess-petri-cancel-001",
				Status:           LifecycleStatusCanceled,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       jssource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusUnavailable)},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusCanceled),
			},
			{
				SessionID:        "dur-sess-petri-run-001",
				Status:           LifecycleStatusRunning,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       jssource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusNotReady)},
				Progress:      &ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusRunning),
			},
		},
	}

	liveOnly := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopeLive})
	if len(liveOnly.LiveSessions) != 3 || len(liveOnly.DurableSessions) != 0 {
		t.Fatalf("live scope = %#v, want live rows only", liveOnly)
	}
	if liveOnly.LiveSessions[0].ID != "dur-sess-petri-success-001" ||
		liveOnly.LiveSessions[1].ID != "live-alpha" ||
		liveOnly.LiveSessions[2].ID != "live-zeta" {
		t.Fatalf("live sort = %#v, want stable id order", liveOnly.LiveSessions)
	}

	persistedOnly := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopePersisted})
	if len(persistedOnly.LiveSessions) != 0 {
		t.Fatalf("persisted live rows = %#v, want none", persistedOnly.LiveSessions)
	}
	if len(persistedOnly.DurableSessions) != 2 {
		t.Fatalf("persisted durable rows = %#v, want terminal/interrupted only", persistedOnly.DurableSessions)
	}
	if persistedOnly.DurableSessions[0].SessionID != "dur-sess-petri-cancel-001" ||
		persistedOnly.DurableSessions[1].SessionID != "dur-sess-petri-success-001" {
		t.Fatalf("persisted sort = %#v, want stable session-id order", persistedOnly.DurableSessions)
	}

	allScope := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopeAll})
	if len(allScope.LiveSessions) != 2 {
		t.Fatalf("all live rows = %#v, want deduped live rows", allScope.LiveSessions)
	}
	for _, session := range allScope.LiveSessions {
		if session.ID == "dur-sess-petri-success-001" {
			t.Fatalf("all scope still contains deduped live id %q", session.ID)
		}
	}
	if len(allScope.DurableSessions) != 3 {
		t.Fatalf("all durable rows = %#v, want full durable list", allScope.DurableSessions)
	}
}

func TestMatchesDurableSessionListFilters_StatusOrchestratorAndRecoverability(t *testing.T) {
	summary := DurableSessionListSummary{
		SessionID:        "dur-sess-js-interrupted-001",
		Status:           LifecycleStatusInterrupted,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: ResolvedSource{
			Kind:      jssource.KindWorkflowFile,
			SourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
		},
		Recoverable: true,
		StaleLease:  true,
	}

	if !MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses:          []LifecycleStatus{LifecycleStatusInterrupted},
		OrchestratorKinds: []string{"javascript"},
		SourceKind:        jssource.KindWorkflowFile,
		SourceRef:         "docs-refresh",
		Recoverable:       boolPtr(true),
		StaleLease:        boolPtr(true),
	}) {
		t.Fatal("expected summary to match recoverable durable filters")
	}

	if MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses: []LifecycleStatus{LifecycleStatusSucceeded},
	}) {
		t.Fatal("expected status filter mismatch")
	}
}

func TestDurableListSummaryFromSessionRead_ProjectsActionAvailability(t *testing.T) {
	read := SessionReadResult{
		SessionID:        "dur-sess-petri-run-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "PETRI",
		Progress:         &ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
		ResultSummary:    &ResultSummary{ResultStatus: string(ResultStatusNotReady)},
		ArtifactRefs: []ArtifactRefSummary{
			{ID: "art-001"},
		},
	}
	summary := DurableListSummaryFromSessionRead(read)
	if summary.ArtifactCount != 1 {
		t.Fatalf("artifactCount = %d, want 1", summary.ArtifactCount)
	}
	if !summary.Actions.CanPause || summary.Actions.CanRetryDispatch {
		t.Fatalf("actions = %#v, want pause without retry", summary.Actions)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
