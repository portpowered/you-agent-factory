package fixtures_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
)

func TestFakeService_PublishedScenarios_ListSessionsScopedWithDedup(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)

	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("fse.StartAsync running: %v", err)
	}
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("fse.StartSync success: %v", err)
	}

	live, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{Scope: fse.SessionListScopeLive})
	if err != nil {
		t.Fatalf("fse.ListSessions live: %v", err)
	}
	if len(live.DurableSessions) != 0 {
		t.Fatalf("live durable rows = %#v, want none", live.DurableSessions)
	}
	if !containsLiveSessionID(live.LiveSessions, runningRow.SessionID) {
		t.Fatalf("live sessions = %#v, want running row %q", live.LiveSessions, runningRow.SessionID)
	}
	if !containsLiveSessionID(live.LiveSessions, successRow.SessionID) {
		t.Fatalf("live sessions = %#v, want terminal row %q", live.LiveSessions, successRow.SessionID)
	}

	persisted, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{Scope: fse.SessionListScopePersisted})
	if err != nil {
		t.Fatalf("fse.ListSessions persisted: %v", err)
	}
	if containsDurableSessionID(persisted.DurableSessions, runningRow.SessionID) {
		t.Fatalf("persisted rows unexpectedly contain running session %q", runningRow.SessionID)
	}
	if !containsDurableSessionID(persisted.DurableSessions, successRow.SessionID) {
		t.Fatalf("persisted rows = %#v, want terminal row %q", persisted.DurableSessions, successRow.SessionID)
	}

	all, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{Scope: fse.SessionListScopeAll})
	if err != nil {
		t.Fatalf("fse.ListSessions all: %v", err)
	}
	if containsLiveSessionID(all.LiveSessions, successRow.SessionID) {
		t.Fatalf("all-scope live rows still contain deduped id %q", successRow.SessionID)
	}
	if !containsLiveSessionID(all.LiveSessions, runningRow.SessionID) {
		t.Fatalf("all-scope live rows = %#v, want running row %q", all.LiveSessions, runningRow.SessionID)
	}
	if !containsDurableSessionID(all.DurableSessions, successRow.SessionID) {
		t.Fatalf("all-scope durable rows = %#v, want terminal row %q", all.DurableSessions, successRow.SessionID)
	}
}

func TestNormalizeListSessionsRequest_DefaultsToLiveAndRejectsUnsupportedScope(t *testing.T) {
	normalized, err := fse.NormalizeListSessionsRequest(fse.ListSessionsRequest{})
	if err != nil {
		t.Fatalf("fse.NormalizeListSessionsRequest: %v", err)
	}
	if normalized.Scope != fse.SessionListScopeLive {
		t.Fatalf("scope = %q, want live", normalized.Scope)
	}

	_, err = fse.NormalizeListSessionsRequest(fse.ListSessionsRequest{Scope: fse.SessionListScope("workspace")})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestApplySessionListScope_LivePersistedAndAllDedup(t *testing.T) {
	startedAt := time.Date(2026, 6, 8, 10, 0, 1, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	base := fse.ListSessionsResult{
		LiveSessions: []fse.LiveSessionSummary{
			{ID: "live-alpha", Project: "alpha"},
			{ID: "dur-sess-petri-success-001", Project: "alpha"},
			{ID: "live-zeta", Project: "zeta"},
		},
		DurableSessions: []fse.DurableSessionListSummary{
			{
				SessionID:        "dur-sess-petri-success-001",
				Status:           fse.LifecycleStatusSucceeded,
				OrchestratorKind: "PETRI",
				ResolvedSource: fse.ResolvedSource{
					Kind:       factory.WorkflowSourceKindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &fse.ResultSummary{ResultStatus: string(fse.ResultStatusFinal)},
				Progress:      &fse.ProgressCounts{TotalDispatches: 1, CompletedDispatches: 1},
				ArtifactCount: 1,
				Lifecycle:     &fse.LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
				Actions:       fse.DeriveSessionActionAvailability(fse.LifecycleStatusSucceeded),
			},
			{
				SessionID:        "dur-sess-petri-cancel-001",
				Status:           fse.LifecycleStatusCanceled,
				OrchestratorKind: "PETRI",
				ResolvedSource: fse.ResolvedSource{
					Kind:       factory.WorkflowSourceKindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &fse.ResultSummary{ResultStatus: string(fse.ResultStatusUnavailable)},
				Actions:       fse.DeriveSessionActionAvailability(fse.LifecycleStatusCanceled),
			},
			{
				SessionID:        "dur-sess-petri-run-001",
				Status:           fse.LifecycleStatusRunning,
				OrchestratorKind: "PETRI",
				ResolvedSource: fse.ResolvedSource{
					Kind:       factory.WorkflowSourceKindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &fse.ResultSummary{ResultStatus: string(fse.ResultStatusNotReady)},
				Progress:      &fse.ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
				Actions:       fse.DeriveSessionActionAvailability(fse.LifecycleStatusRunning),
			},
		},
	}

	liveOnly := fse.ApplySessionListScope(base, fse.ListSessionsRequest{Scope: fse.SessionListScopeLive})
	if len(liveOnly.LiveSessions) != 3 || len(liveOnly.DurableSessions) != 0 {
		t.Fatalf("live scope = %#v, want live rows only", liveOnly)
	}
	if liveOnly.LiveSessions[0].ID != "dur-sess-petri-success-001" ||
		liveOnly.LiveSessions[1].ID != "live-alpha" ||
		liveOnly.LiveSessions[2].ID != "live-zeta" {
		t.Fatalf("live sort = %#v, want stable id order", liveOnly.LiveSessions)
	}

	persistedOnly := fse.ApplySessionListScope(base, fse.ListSessionsRequest{Scope: fse.SessionListScopePersisted})
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

	allScope := fse.ApplySessionListScope(base, fse.ListSessionsRequest{Scope: fse.SessionListScopeAll})
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
