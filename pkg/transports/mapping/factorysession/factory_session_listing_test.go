package factorysession_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestScopedSessionListResponseToAPIMapsDetachedOwnerProjectionOnly(t *testing.T) {
	t.Parallel()

	result := factorysession.ScopedSessionListResponseToAPI(factorysessions.ScopedSessionListResult{
		Scope: factorysessions.SessionListScopeAll,
		LiveSessions: []factorysessions.ScopedLiveSessionSummary{{
			ID: "live-1", FactoryDir: "/factory", FolderPath: "/workspace", Project: "project",
			Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "goal"},
			Runtime: &factorysessions.RuntimeProjection{Status: "RUNNING"},
		}},
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: "durable-1", Status: factorysessions.LifecycleStatusPaused,
			Actions: factorysessions.SessionActionAvailability{CanResume: true},
		}},
	})
	if len(result.Sessions) != 1 || result.Sessions[0].Runtime == nil ||
		result.Sessions[0].Target.Name == nil || *result.Sessions[0].Target.Name != "goal" {
		t.Fatalf("live response = %#v, want detached runtime and target", result.Sessions)
	}
	if result.DurableSessions == nil || len(*result.DurableSessions) != 1 ||
		(*result.DurableSessions)[0].Actions == nil || (*result.DurableSessions)[0].Actions.CanResume == nil ||
		!*(*result.DurableSessions)[0].Actions.CanResume {
		t.Fatalf("durable response = %#v, want owner-projected actions", result.DurableSessions)
	}
}

func TestLogicalTargetToAPI_DefaultNamedAndProvider(t *testing.T) {
	t.Parallel()

	defaultRef := factorysessions.CanonicalLogicalTargetReference{
		BackendScopeID: "scope-1",
		FolderPath:     "/workspace",
		Kind:           factorysessions.LogicalTargetKindDefault,
	}
	defaultTarget := factorysession.LogicalTargetToAPI(defaultRef)
	if defaultTarget.Kind != factoryapi.FactorySessionLogicalTargetKindDefault {
		t.Fatalf("default kind = %q", defaultTarget.Kind)
	}
	if defaultTarget.NamedTarget != nil || defaultTarget.ProviderBoundary != nil {
		t.Fatalf("default target should not include named or provider fields: %#v", defaultTarget)
	}

	namedRef := factorysessions.CanonicalLogicalTargetReference{
		BackendScopeID: "scope-1",
		FolderPath:     "/workspace",
		Kind:           factorysessions.LogicalTargetKindNamed,
		NamedTarget:    "goal",
	}
	namedTarget := factorysession.LogicalTargetToAPI(namedRef)
	if namedTarget.Kind != factoryapi.FactorySessionLogicalTargetKindNamed {
		t.Fatalf("named kind = %q", namedTarget.Kind)
	}
	if namedTarget.NamedTarget == nil || *namedTarget.NamedTarget != namedRef.NamedTarget {
		t.Fatalf("namedTarget = %#v, want %q", namedTarget.NamedTarget, namedRef.NamedTarget)
	}

	providerRef := factorysessions.CanonicalLogicalTargetReference{
		BackendScopeID: "scope-1",
		FolderPath:     "/workspace",
		Kind:           factorysessions.LogicalTargetKindProvider,
		Provider: &factorysessions.LogicalTargetProviderBoundary{
			Provider: "cursor",
			Kind:     "agent",
			Boundary: "workspace-1",
		},
	}
	providerTarget := factorysession.LogicalTargetToAPI(providerRef)
	if providerTarget.Kind != factoryapi.FactorySessionLogicalTargetKindProvider {
		t.Fatalf("provider kind = %q", providerTarget.Kind)
	}
	if providerTarget.ProviderBoundary == nil || providerTarget.ProviderBoundary.Boundary != "workspace-1" {
		t.Fatalf("provider boundary = %#v", providerTarget.ProviderBoundary)
	}
}

func TestOpenRequestFromAPI_DetachesAndNormalizesPublicInput(t *testing.T) {
	t.Parallel()

	name := " beta "
	validateOnly := true
	request := factoryapi.OpenFactorySessionRequest{
		FolderPath:   "/workspace",
		Target:       &factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindNamed, Name: &name},
		ValidateOnly: &validateOnly,
	}

	mapped := factorysession.OpenRequestFromAPI(request)
	name = "changed"
	validateOnly = false

	if mapped.FolderPath != "/workspace" || !mapped.ValidateOnly || mapped.InitNewFactory {
		t.Fatalf("open request = %#v, want detached validate-only request", mapped)
	}
	if mapped.Target == nil || mapped.Target.Kind != factorysessions.TargetKindNamed || mapped.Target.Name != "beta" {
		t.Fatalf("open target = %#v, want trimmed named target", mapped.Target)
	}
}

func TestOpenResultToAPI_PreservesHintsTargetsAndSession(t *testing.T) {
	t.Parallel()

	result := &factorysessions.OpenResult{
		FolderPath:      " /workspace ",
		InitsNewFactory: true,
		SessionID:       "session-beta",
		Targets: []factorysessions.Target{{
			Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
		}},
	}
	session := &factorysessions.LiveSession{
		ID: "session-beta",
		SessionState: factorysessions.SessionState{
			FactoryDir:       "/workspace/factory/beta",
			FolderPath:       "/workspace",
			ExecutionBaseDir: "/workspace",
		},
		Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
		Project: "demo",
	}

	response := factorysession.OpenResultToAPI(result, session)
	if response.InitsNewFactory == nil || !*response.InitsNewFactory ||
		response.FolderPath == nil || *response.FolderPath != "/workspace" {
		t.Fatalf("open hints = %#v, want init hint and trimmed folder", response)
	}
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Ref.Name == nil || *(*response.Targets)[0].Ref.Name != "beta" {
		t.Fatalf("targets = %#v, want named beta", response.Targets)
	}
	if response.Session == nil || response.Session.Id != "session-beta" || response.Session.Project != "demo" {
		t.Fatalf("session = %#v, want mapped session-beta", response.Session)
	}
}

func TestSyncPreflightResultToAPI_PreservesReconnectDecisionAndIdentity(t *testing.T) {
	t.Parallel()

	afterEventID := "event-7"
	afterSequence := int64(7)
	backendScopeID := "backend-1"
	factorySessionID := "session-1"
	logicalSessionKeyID := "/tmp/demo::default::"
	streamGenerationID := "backend-1::session-1"

	response := factorysession.SyncPreflightResultToAPI(factorysessions.SyncPreflightResult{
		BackendScopeID:      &backendScopeID,
		CheckpointReusable:  true,
		FactorySessionID:    &factorySessionID,
		LogicalSessionKeyID: &logicalSessionKeyID,
		Reason:              factorysessions.SyncPreflightReasonOK,
		ReconnectCursor: factorysessions.SyncPreflightReconnectCursor{
			AfterEventID:             &afterEventID,
			AfterSequence:            &afterSequence,
			Provided:                 true,
			ValidForStreamGeneration: true,
		},
		RequestedSessionID: "~default",
		StreamGenerationID: &streamGenerationID,
	})

	if response.ReasonCode != factoryapi.Ok || !response.CheckpointReusable {
		t.Fatalf("preflight decision = %#v, want reusable ok", response)
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != factorySessionID {
		t.Fatalf("factorySessionId = %#v, want %q", response.FactorySessionId, factorySessionID)
	}
	if response.ReconnectCursor.AfterEventId == nil || *response.ReconnectCursor.AfterEventId != afterEventID ||
		response.ReconnectCursor.AfterSequence == nil || *response.ReconnectCursor.AfterSequence != afterSequence ||
		!response.ReconnectCursor.Provided || !response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("reconnectCursor = %#v, want acknowledged valid cursor", response.ReconnectCursor)
	}
}

func TestSessionSummaryAndTargetsToAPI_PreservePublicFieldsAndOrdering(t *testing.T) {
	t.Parallel()

	defaultSession := &factorysessions.LiveSession{
		ID: factorysessions.DefaultSessionID,
		SessionState: factorysessions.SessionState{
			FactoryDir:       "/factories/default",
			FolderPath:       "/workspace",
			ExecutionBaseDir: "/workspace",
		},
		Target:                  factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		IsDefault:               true,
		Project:                 "default-project",
		RuntimeFactorySessionID: "11111111-1111-4111-8111-111111111111",
	}
	namedSession := &factorysessions.LiveSession{
		ID: "session-beta",
		SessionState: factorysessions.SessionState{
			FactoryDir:       "/factories/beta",
			FolderPath:       "/workspace",
			ExecutionBaseDir: "/workspace",
		},
		Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: " beta "},
		Project: "beta-project",
	}
	summaries := []factoryapi.FactorySessionSummary{
		factorysession.SessionSummaryToAPI(defaultSession),
		factorysession.SessionSummaryToAPI(namedSession),
	}
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

func TestReadProjectionsToAPI_PreservesRuntimeAvailability(t *testing.T) {
	t.Parallel()

	withRuntime := &factorysessions.LiveSession{ID: "session-runtime", Project: "runtime-project"}
	fallback := &factorysessions.LiveSession{ID: "session-fallback", Project: "fallback-project"}
	response := factorysession.ReadProjectionsToAPI([]factorysessions.ReadProjection{
		{
			Context:          factorysessions.ProjectionContext{Session: withRuntime},
			Runtime:          factorysessions.RuntimeProjection{Status: "IDLE"},
			RuntimeAvailable: true,
		},
		{Context: factorysessions.ProjectionContext{Session: fallback}},
	})

	if len(response.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(response.Sessions))
	}
	if response.Sessions[0].Id != "session-runtime" || response.Sessions[0].Runtime == nil ||
		response.Sessions[0].Runtime.Status != factoryapi.FactorySessionStatusIDLE {
		t.Fatalf("runtime summary = %#v, want mapped IDLE runtime", response.Sessions[0])
	}
	if response.Sessions[1].Id != "session-fallback" || response.Sessions[1].Runtime != nil {
		t.Fatalf("fallback summary = %#v, want identity without runtime", response.Sessions[1])
	}
}

func TestLogicalTargetFromSession_NilNamedAndInvalid(t *testing.T) {
	t.Parallel()

	normalize := func(
		backendScopeID string,
		folderPath string,
		ref factorysessions.TargetRef,
	) (factorysessions.CanonicalLogicalTargetReference, error) {
		if ref.Kind == factorysessions.TargetKindNamed && ref.Name == "" {
			return factorysessions.CanonicalLogicalTargetReference{}, errors.New("named target is required")
		}
		return factorysessions.CanonicalLogicalTargetReference{
			BackendScopeID: backendScopeID,
			FolderPath:     folderPath,
			Kind:           factorysessions.LogicalTargetKindNamed,
			NamedTarget:    ref.Name,
		}, nil
	}

	target, err := factorysession.LogicalTargetFromSession(normalize, "scope-1", nil)
	if err != nil || target != nil {
		t.Fatalf("LogicalTargetFromSession(nil) = (%#v, %v), want nil,nil", target, err)
	}

	session := &factorysessions.LiveSession{
		SessionState: factorysessions.SessionState{FolderPath: t.TempDir()},
		Target:       factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "goal"},
	}
	target, err = factorysession.LogicalTargetFromSession(normalize, "scope-1", session)
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
	if _, err := factorysession.LogicalTargetFromSession(normalize, "scope-1", invalidSession); err == nil {
		t.Fatal("LogicalTargetFromSession(invalid named target) = nil, want validation error")
	}
}

func TestListSessionsRequestFromAPI_DefaultsToLiveScope(t *testing.T) {
	request, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{})
	if err != nil {
		t.Fatalf("ListSessionsRequestFromAPI: %v", err)
	}
	if request.Scope != "" {
		t.Fatalf("scope = %q, want raw omitted value", request.Scope)
	}
}

func TestListSessionsRequestFromAPI_RejectsUnsupportedScope(t *testing.T) {
	scope := factoryapi.FactorySessionListScope("workspace")
	raw, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &scope})
	if err != nil {
		t.Fatalf("ListSessionsRequestFromAPI: %v", err)
	}
	if raw.Scope != factorysessionexecution.SessionListScope("workspace") {
		t.Fatalf("scope = %q, want raw unsupported value", raw.Scope)
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
					Kind:       factory.WorkflowSourceKindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &factorysessionexecution.ResultSummary{ResultStatus: "FINAL"},
				Lifecycle:     &factorysessionexecution.LifecycleTimestamps{StartedAt: startedAt, FinishedAt: finishedAt},
				Actions:       factorysessionexecution.SessionActionAvailability{},
			},
		},
	}

	persisted := factorysession.ListSessionsResponseToAPI(
		factorysessionexecution.ListSessionsResult{
			Scope:           factorysessionexecution.SessionListScopePersisted,
			DurableSessions: base.DurableSessions,
		},
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
		factorysessionexecution.ListSessionsResult{
			Scope:           factorysessionexecution.SessionListScopeAll,
			LiveSessions:    base.LiveSessions[:1],
			DurableSessions: base.DurableSessions,
		},
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
