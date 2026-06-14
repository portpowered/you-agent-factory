package factorysession_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestMockClient_ListDispatches_DispatchInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := newFixtureMCPClient(t)
	row := publishedScenario(t, fixtures.FixturePurposeDispatchInspection)

	if _, err := client.StartSync(syncSuccessExecutionRequest()); err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	response, err := client.ListDispatches(mcpfactorysession.ListDispatchesInput{
		SessionID: row.SessionID,
	})
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want dispatch list", response)
	}
	if response.Result.SessionId != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.Result.SessionId, row.SessionID)
	}
	if len(response.Result.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one row", response.Result.Dispatches)
	}
	dispatch := response.Result.Dispatches[0]
	if dispatch.Id != "disp-petri-success-001" {
		t.Fatalf("dispatch.id = %q, want disp-petri-success-001", dispatch.Id)
	}
	if dispatch.Status == "" || dispatch.DispatchKind == "" {
		t.Fatalf("dispatch missing status/kind: %#v", dispatch)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartSync(context.Background(), syncSuccessStartRequest()); err != nil {
		t.Fatalf("direct StartSync: %v", err)
	}
	listed, err := service.ListDispatches(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("direct ListDispatches: %v", err)
	}
	wantHash, err := fixtures.ListDispatchesResultHash(listed)
	if err != nil {
		t.Fatalf("fixtures.ListDispatchesResultHash: %v", err)
	}
	if wantHash != "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP artifact inspection test keeps fixture summaries and golden-hash assertions together on one mock-client seam.
func TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := newFixtureMCPClient(t)
	row := publishedScenario(t, fixtures.FixturePurposeArtifactInspection)

	started, err := client.StartAsync(artifactInspectionExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	response, err := client.ListArtifacts(mcpfactorysession.ListArtifactsInput{
		SessionID: row.SessionID,
	})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want artifact list", response)
	}
	if response.Result.SessionId != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.Result.SessionId, row.SessionID)
	}
	if len(response.Result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one row", response.Result.Artifacts)
	}
	artifact := response.Result.Artifacts[0]
	if artifact.Id != "art-js-pause-001" {
		t.Fatalf("artifact.id = %q, want art-js-pause-001", artifact.Id)
	}
	if artifact.Kind == "" {
		t.Fatalf("artifact missing kind: %#v", artifact)
	}
	if artifact.DispatchId == nil || *artifact.DispatchId != "disp-js-pause-001" {
		t.Fatalf("dispatchId = %#v, want disp-js-pause-001", artifact.DispatchId)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), artifactInspectionStartRequest()); err != nil {
		t.Fatalf("direct StartAsync: %v", err)
	}
	listed, err := service.ListArtifacts(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("direct ListArtifacts: %v", err)
	}
	wantHash, err := fixtures.ListArtifactsResultHash(listed)
	if err != nil {
		t.Fatalf("fixtures.ListArtifactsResultHash: %v", err)
	}
	if wantHash != "sha256:57fa7af131ce29cb2a254d2548ef8b8f9b0ccf6de7fb6cc185beabf8190f1dcb" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

func TestMockClient_ListArtifacts_WorkflowAliasParity(t *testing.T) {
	canonicalClient := newFixtureMCPClient(t)
	aliasClient := newFixtureMCPClient(t)
	row := publishedScenario(t, fixtures.FixturePurposeArtifactInspection)

	for _, client := range []*mcpfactorysession.Client{canonicalClient, aliasClient} {
		started, err := client.StartAsync(artifactInspectionExecutionRequest())
		if err != nil {
			t.Fatalf("StartAsync: %v", err)
		}
		if started.Error != nil || started.Result == nil {
			t.Fatalf("start = %#v, want success", started)
		}
	}

	canonicalRaw, err := canonicalClient.CallTool(
		mcpfactorysession.ToolListArtifacts,
		mustJSON(t, mcpfactorysession.ListArtifactsInput{SessionID: row.SessionID}),
	)
	if err != nil {
		t.Fatalf("canonical CallTool: %v", err)
	}
	aliasRaw, err := aliasClient.CallTool(
		mcpfactorysession.ToolWorkflowArtifacts,
		mustJSON(t, mcpfactorysession.ListArtifactsInput{SessionID: row.SessionID}),
	)
	if err != nil {
		t.Fatalf("alias CallTool: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias parity drift:\ncanonical=%s\nalias=%s", canonicalRaw, aliasRaw)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP event inspection test keeps canonical event vocabulary and golden-hash assertions together on one mock-client seam.
func TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents(t *testing.T) {
	client := newFixtureMCPClient(t)
	row := publishedScenario(t, fixtures.FixturePurposeEventReconnect)

	started, err := client.StartAsync(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	response, err := client.ReadEvents(mcpfactorysession.ReadEventsInput{
		SessionID: row.SessionID,
	})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want event read", response)
	}
	if response.Result.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.Result.SessionID, row.SessionID)
	}
	if len(response.Result.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(response.Result.Events))
	}
	if response.Result.Events[0].Type != "SESSION_STARTED" {
		t.Fatalf("first event type = %q, want SESSION_STARTED", response.Result.Events[0].Type)
	}
	for _, event := range response.Result.Events {
		if strings.Contains(strings.ToUpper(string(event.Type)), "PETRI") {
			t.Fatalf("event type exposes internal vocabulary: %q", event.Type)
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != row.SessionID {
			t.Fatalf("event context sessionId = %#v, want %q", event.Context.SessionId, row.SessionID)
		}
	}

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), asyncRunningStartRequest()); err != nil {
		t.Fatalf("direct StartAsync: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), row.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("direct ReadEvents: %v", err)
	}
	wantHash, err := fixtures.EventReadResultHash(events)
	if err != nil {
		t.Fatalf("fixtures.EventReadResultHash: %v", err)
	}
	if wantHash != "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP lifecycle control test keeps accepted, rejected, and session-isolation assertions together on one mock-client seam.
func TestMockClient_Control_LifecycleFixtureReturnsAcceptedRejectedAndIsolatesSessions(t *testing.T) {
	client := newFixtureMCPClient(t)
	pausedRow := publishedScenario(t, fixtures.FixturePurposeLifecycleControl)
	runningRow := publishedScenario(t, fixtures.FixturePurposeAsyncRunning)
	terminalRow := publishedScenario(t, fixtures.FixturePurposeSyncSuccess)

	if _, err := client.StartAsync(artifactInspectionExecutionRequest()); err != nil {
		t.Fatalf("StartAsync paused: %v", err)
	}
	if _, err := client.StartAsync(asyncRunningExecutionRequest()); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	if _, err := client.StartSync(syncSuccessExecutionRequest()); err != nil {
		t.Fatalf("StartSync terminal: %v", err)
	}

	noOp, err := client.Control(mcpfactorysession.ControlInput{
		SessionID: pausedRow.SessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
	})
	if err != nil {
		t.Fatalf("Control pause no-op: %v", err)
	}
	if noOp.Error != nil || noOp.Result == nil {
		t.Fatalf("pause no-op = %#v, want typed control result", noOp)
	}
	if noOp.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("pause outcome = %q, want NO_OP", noOp.Result.Outcome)
	}

	resumed, err := client.Control(mcpfactorysession.ControlInput{
		SessionID: pausedRow.SessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindResume,
	})
	if err != nil {
		t.Fatalf("Control resume: %v", err)
	}
	if resumed.Error != nil || resumed.Result == nil {
		t.Fatalf("resume = %#v, want typed control result", resumed)
	}
	if resumed.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumed.Result.Outcome)
	}
	if resumed.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", resumed.Result.Status)
	}
	resumeHash, err := fixtures.LifecycleControlResultHash(lifecycleControlFromAPI(*resumed.Result))
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash: %v", err)
	}
	if resumeHash != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("resume hash = %q", resumeHash)
	}

	terminated, err := client.Control(mcpfactorysession.ControlInput{
		SessionID: runningRow.SessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindTerminate,
	})
	if err != nil {
		t.Fatalf("Control terminate: %v", err)
	}
	if terminated.Error != nil || terminated.Result == nil {
		t.Fatalf("terminate = %#v, want typed control result", terminated)
	}
	if terminated.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("terminate outcome = %q, want ACCEPTED", terminated.Result.Outcome)
	}

	rejected, err := client.Control(mcpfactorysession.ControlInput{
		SessionID: terminalRow.SessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindCancel,
	})
	if err != nil {
		t.Fatalf("Control cancel terminal: %v", err)
	}
	if rejected.Error != nil || rejected.Result == nil {
		t.Fatalf("terminal cancel = %#v, want typed TERMINAL_SESSION result", rejected)
	}
	if rejected.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("terminal cancel outcome = %q, want TERMINAL_SESSION", rejected.Result.Outcome)
	}

	pausedAfter, err := client.GetSession(mcpfactorysession.GetSessionInput{SessionID: pausedRow.SessionID})
	if err != nil {
		t.Fatalf("GetSession paused after terminate: %v", err)
	}
	if pausedAfter.Error != nil || pausedAfter.Result == nil {
		t.Fatalf("paused read = %#v", pausedAfter)
	}
	if pausedAfter.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("paused session mutated to %q, want RUNNING unchanged", pausedAfter.Result.Status)
	}
}

func publishedScenario(t *testing.T, purpose fixtures.FixtureScenarioPurpose) fixtures.PublishedFixtureScenario {
	t.Helper()
	for _, row := range fixtures.PublishedFixtureScenarios {
		if row.Purpose == purpose {
			return row
		}
	}
	t.Fatalf("published scenario missing for purpose %q", purpose)
	return fixtures.PublishedFixtureScenario{}
}

func artifactInspectionExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-paused-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	}
}

func syncSuccessStartRequest() factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
		Args: map[string]any{"ticketId": "TKT-2002"},
	}
}

func artifactInspectionStartRequest() factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: "req-js-paused-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}
}

func asyncRunningStartRequest() factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: "req-js-run-n-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}
}

func lifecycleControlFromAPI(
	response factoryapi.FactorySessionLifecycleControlResponse,
) factorysessionexecution.LifecycleControlResult {
	result := factorysessionexecution.LifecycleControlResult{
		SessionID: response.SessionId,
		Operation: factorysessionexecution.LifecycleControlKind(response.Operation),
		Outcome:   factorysessionexecution.LifecycleControlOutcome(response.Outcome),
		Status:    factorysessionexecution.LifecycleStatus(response.Status),
	}
	if response.DispatchId != nil {
		result.DispatchID = *response.DispatchId
	}
	if response.RetryDispatchId != nil {
		result.RetryDispatchID = *response.RetryDispatchId
	}
	if response.Links != nil {
		result.Links = factorysessionexecution.LifecycleControlLinks{
			Session:    derefString(response.Links.Session),
			Status:     derefString(response.Links.Status),
			Events:     derefString(response.Links.Events),
			Results:    derefString(response.Links.Results),
			Dispatches: derefString(response.Links.Dispatches),
			Artifacts:  derefString(response.Links.Artifacts),
		}
	}
	return result
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return encoded
}
