package factorysession_test

import (
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/logicaltarget"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestLogicalTargetToAPI_DefaultNamedAndProvider(t *testing.T) {
	t.Parallel()

	defaultRef, err := logicaltarget.NormalizeDefaultTarget("scope-1", t.TempDir())
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}
	defaultTarget := factorysession.LogicalTargetToAPI(defaultRef)
	if defaultTarget.Kind != factoryapi.FactorySessionLogicalTargetKindDefault {
		t.Fatalf("default kind = %q", defaultTarget.Kind)
	}
	if defaultTarget.NamedTarget != nil || defaultTarget.ProviderBoundary != nil {
		t.Fatalf("default target should not include named or provider fields: %#v", defaultTarget)
	}

	namedRef, err := logicaltarget.NormalizeNamedTarget("scope-1", t.TempDir(), "goal")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}
	namedTarget := factorysession.LogicalTargetToAPI(namedRef)
	if namedTarget.Kind != factoryapi.FactorySessionLogicalTargetKindNamed {
		t.Fatalf("named kind = %q", namedTarget.Kind)
	}
	if namedTarget.NamedTarget == nil || *namedTarget.NamedTarget != namedRef.NamedTarget {
		t.Fatalf("namedTarget = %#v, want %q", namedTarget.NamedTarget, namedRef.NamedTarget)
	}

	providerRef, err := logicaltarget.NormalizeProviderTarget("scope-1", t.TempDir(), logicaltarget.ProviderBoundary{
		Provider: "cursor", Kind: "agent", Boundary: "workspace-1",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderTarget: %v", err)
	}
	providerTarget := factorysession.LogicalTargetToAPI(providerRef)
	if providerTarget.Kind != factoryapi.FactorySessionLogicalTargetKindProvider {
		t.Fatalf("provider kind = %q", providerTarget.Kind)
	}
	if providerTarget.ProviderBoundary == nil || providerTarget.ProviderBoundary.Boundary != "workspace-1" {
		t.Fatalf("provider boundary = %#v", providerTarget.ProviderBoundary)
	}
}

func TestSessionSummaryAndTargetsToAPI_PreservePublicFieldsAndOrdering(t *testing.T) {
	t.Parallel()

	defaultSession := factorysessions.NewLiveSession(
		factorysessions.DefaultSessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		nil,
		true,
		"default-project",
	)
	namedSession := factorysessions.NewLiveSession(
		"session-beta",
		"/factories/beta",
		"/workspace",
		"/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: " beta "},
		nil,
		false,
		"beta-project",
	)
	summaries := []factoryapi.FactorySessionSummary{
		factorysession.SessionSummaryToAPI(namedSession),
		factorysession.SessionSummaryToAPI(defaultSession),
	}
	factorysession.SortSessionSummaries(summaries)
	if !summaries[0].IsDefault || summaries[0].Id != factorysessions.CanonicalFactorySessionID(defaultSession) {
		t.Fatalf("first summary = %#v, want canonical default session first", summaries[0])
	}
	if summaries[1].Project != "beta-project" || summaries[1].Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		summaries[1].Target.Name == nil || *summaries[1].Target.Name != "beta" {
		t.Fatalf("named summary = %#v, want detached public target fields", summaries[1])
	}

	targets := factorysession.TargetsToAPI([]factorysessions.Target{{
		FactoryDir: "/factories/beta",
		FolderPath: "/workspace",
		Label:      "Beta",
		Project:    "beta-project",
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: " beta "},
	}})
	if len(targets) != 1 || targets[0].Ref.Name == nil || *targets[0].Ref.Name != "beta" || targets[0].Label != "Beta" {
		t.Fatalf("targets = %#v, want mapped named target", targets)
	}
}

func TestLogicalTargetFromSession_NilNamedAndInvalid(t *testing.T) {
	t.Parallel()

	target, err := factorysession.LogicalTargetFromSession("scope-1", nil)
	if err != nil || target != nil {
		t.Fatalf("LogicalTargetFromSession(nil) = (%#v, %v), want nil,nil", target, err)
	}

	session := &factorysessions.LiveSession{
		SessionState: factorysessions.SessionState{FolderPath: t.TempDir()},
		Target:       factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "goal"},
	}
	target, err = factorysession.LogicalTargetFromSession("scope-1", session)
	if err != nil {
		t.Fatalf("LogicalTargetFromSession: %v", err)
	}
	if target == nil || target.Kind != factoryapi.FactorySessionLogicalTargetKindNamed {
		t.Fatalf("target = %#v", target)
	}

	invalidSession := &factorysessions.LiveSession{
		SessionState: factorysessions.SessionState{FolderPath: t.TempDir()},
		Target:       factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed},
	}
	if _, err := factorysession.LogicalTargetFromSession("scope-1", invalidSession); err == nil {
		t.Fatal("LogicalTargetFromSession(invalid named target) = nil, want validation error")
	}
}

func TestListSessionsRequestFromAPI_DefaultsToLiveScope(t *testing.T) {
	request, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{})
	if err != nil {
		t.Fatalf("ListSessionsRequestFromAPI: %v", err)
	}
	if request.Scope != factorysessionexecution.SessionListScopeLive {
		t.Fatalf("scope = %q, want live", request.Scope)
	}
}

func TestListSessionsRequestFromAPI_RejectsUnsupportedScope(t *testing.T) {
	scope := factoryapi.FactorySessionListScope("workspace")
	_, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &scope})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestDurableSessionSummaryToAPI_MapsListSummaryFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	scenario := findScenario(t, catalog, "petri-succeeded-one-dispatch")
	listSummary, ok := scenario["listSummary"].(map[string]any)
	if !ok {
		t.Fatal("missing listSummary fixture")
	}

	mapped := factorysession.DurableSessionSummaryToAPI(durableListSummaryFromFixture(listSummary))
	if mapped.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q", mapped.SessionId)
	}
	if mapped.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", mapped.Status)
	}
	if mapped.OrchestratorKind != factoryapi.PETRI {
		t.Fatalf("orchestratorKind = %q, want PETRI", mapped.OrchestratorKind)
	}
	if mapped.ResolvedSource.SourceRef == nil || *mapped.ResolvedSource.SourceRef != "factory/customer-support-triage" {
		t.Fatalf("resolvedSource = %#v", mapped.ResolvedSource)
	}
	if mapped.ResultSummary == nil || mapped.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", mapped.ResultSummary)
	}
	if mapped.Progress == nil || mapped.Progress.CompletedDispatches == nil || *mapped.Progress.CompletedDispatches != 1 {
		t.Fatalf("progress = %#v, want one completed dispatch", mapped.Progress)
	}
	if mapped.ArtifactCount == nil || *mapped.ArtifactCount != 1 {
		t.Fatalf("artifactCount = %#v, want 1", mapped.ArtifactCount)
	}
}

func TestListSessionsResponseToAPI_ScopedPersistedAndAll(t *testing.T) {
	startedAt := timeValue(map[string]any{"startedAt": "2026-06-08T10:00:01Z"}, "startedAt")
	finishedAt := timeValue(map[string]any{"finishedAt": "2026-06-08T10:05:00Z"}, "finishedAt")
	base := factorysessionexecution.ListSessionsResult{
		LiveSessions: []factorysessionexecution.LiveSessionSummary{
			{ID: "live-alpha", Project: "alpha"},
			{ID: "dur-sess-petri-success-001", Project: "alpha"},
		},
		DurableSessions: []factorysessionexecution.DurableSessionListSummary{
			{
				SessionID:        "dur-sess-petri-success-001",
				Status:           factorysessionexecution.LifecycleStatusSucceeded,
				OrchestratorKind: "PETRI",
				ResolvedSource: factorysessionexecution.ResolvedSource{
					Kind:       workflowsource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &factorysessionexecution.ResultSummary{ResultStatus: string(factorysessionexecution.ResultStatusFinal)},
				Lifecycle:     &factorysessionexecution.LifecycleTimestamps{StartedAt: startedAt, FinishedAt: finishedAt},
				Actions:       factorysessionexecution.DeriveSessionActionAvailability(factorysessionexecution.LifecycleStatusSucceeded),
			},
		},
	}

	persisted := factorysession.ListSessionsResponseToAPI(
		factorysessionexecution.ApplySessionListScope(base, factorysessionexecution.ListSessionsRequest{
			Scope: factorysessionexecution.SessionListScopePersisted,
		}),
	)
	if persisted.Scope == nil || *persisted.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Fatalf("scope = %#v, want persisted", persisted.Scope)
	}
	if len(persisted.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for persisted scope", persisted.Sessions)
	}
	if persisted.DurableSessions == nil || len(*persisted.DurableSessions) != 1 {
		t.Fatalf("durableSessions = %#v, want one persisted row", persisted.DurableSessions)
	}

	all := factorysession.ListSessionsResponseToAPI(
		factorysessionexecution.ApplySessionListScope(base, factorysessionexecution.ListSessionsRequest{
			Scope: factorysessionexecution.SessionListScopeAll,
		}),
	)
	if all.Scope == nil || *all.Scope != factoryapi.FactorySessionListScopeAll {
		t.Fatalf("scope = %#v, want all", all.Scope)
	}
	if len(all.Sessions) != 1 || all.Sessions[0].Id != "live-alpha" {
		t.Fatalf("sessions = %#v, want deduped live rows", all.Sessions)
	}
	if all.DurableSessions == nil || len(*all.DurableSessions) != 1 {
		t.Fatalf("durableSessions = %#v, want durable rows preserved", all.DurableSessions)
	}
}

func durableListSummaryFromFixture(summary map[string]any) factorysessionexecution.DurableSessionListSummary {
	row := factorysessionexecution.DurableSessionListSummary{
		SessionID:        stringValue(summary, "sessionId"),
		Status:           factorysessionexecution.LifecycleStatus(stringValue(summary, "status")),
		OrchestratorKind: stringValue(summary, "orchestratorKind"),
		Dialect:          stringValue(summary, "dialect"),
		SourceHash:       stringValue(summary, "sourceHash"),
	}
	if resolved, ok := summary["resolvedSource"].(map[string]any); ok {
		row.ResolvedSource = resolvedSourceFromFixture(resolved)
	}
	if requested, ok := summary["requestedPolicy"].(map[string]any); ok {
		row.Policy.Requested = cloneFixtureMap(requested)
	}
	if effective, ok := summary["effectivePolicy"].(map[string]any); ok {
		row.Policy.Effective = cloneFixtureMap(effective)
	}
	row.Policy.EffectiveHash = stringValue(summary, "effectivePolicyHash")
	if lifecycle, ok := summary["lifecycle"].(map[string]any); ok {
		row.Lifecycle = lifecycleTimestampsFromFixture(lifecycle)
	}
	if links, ok := summary["links"].(map[string]any); ok {
		row.Links = inspectionLinksFromFixture(links)
	}
	if progress, ok := summary["progress"].(map[string]any); ok {
		row.Progress = &factorysessionexecution.ProgressCounts{
			TotalDispatches:     intValue(progress, "totalDispatches"),
			CompletedDispatches: intValue(progress, "completedDispatches"),
			FailedDispatches:    intValue(progress, "failedDispatches"),
			InFlightDispatches:  intValue(progress, "inFlightDispatches"),
			PhaseCount:          intValue(progress, "phaseCount"),
		}
	}
	if resultSummary, ok := summary["resultSummary"].(map[string]any); ok {
		row.ResultSummary = &factorysessionexecution.ResultSummary{
			ResultStatus: stringValue(resultSummary, "resultStatus"),
			Summary:      stringValue(resultSummary, "summary"),
		}
	}
	if artifactCount, ok := summary["artifactCount"].(float64); ok {
		row.ArtifactCount = int(artifactCount)
	}
	if recoverable, ok := summary["recoverable"].(bool); ok {
		row.Recoverable = recoverable
	}
	if actions, ok := summary["actions"].(map[string]any); ok {
		row.Actions = sessionActionsFromFixture(actions)
	} else {
		row.Actions = factorysessionexecution.DeriveSessionActionAvailability(row.Status)
	}
	if !row.Recoverable {
		row.Recoverable = factorysessionexecution.IsRecoverableSession(row.Status, row.StaleLease)
	}
	return row
}

func sessionActionsFromFixture(actions map[string]any) factorysessionexecution.SessionActionAvailability {
	return factorysessionexecution.SessionActionAvailability{
		CanPause:             boolValue(actions, "canPause"),
		CanResume:            boolValue(actions, "canResume"),
		CanCancel:            boolValue(actions, "canCancel"),
		CanTerminate:         boolValue(actions, "canTerminate"),
		CanApprove:           boolValue(actions, "canApprove"),
		CanRetryDispatch:     boolValue(actions, "canRetryDispatch"),
		CanInterruptDispatch: boolValue(actions, "canInterruptDispatch"),
	}
}
