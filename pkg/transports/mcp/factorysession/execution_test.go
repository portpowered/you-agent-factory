package factorysession_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/testharness"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

func TestMockClient_StartAsync_RunningFixtureReturnsInProgressSession(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := asyncRunningExecutionRequest()

	response, err := client.StartAsync(request)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want async execution response")
	}
	if response.Result.SessionId != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Result.Status)
	}
	if response.Result.Links == nil || response.Result.Links.Session == nil {
		t.Fatal("links.session missing from async start response")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP running-status test keeps read-model and direct-service assertions together on one mock-client seam.
func TestMockClient_GetSession_RunningFixtureReturnsDeterministicStatus(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	response, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: started.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("read = %#v, want running session read model", response)
	}
	if response.Result.SessionId != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Result.Status)
	}
	if response.Result.Progress == nil || response.Result.Progress.InFlightDispatches == nil {
		t.Fatal("progress.inFlightDispatches missing from running session read model")
	}
	if response.Result.ResultSummary != nil &&
		response.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusPartial {
		t.Fatalf("resultSummary = %#v, want PARTIAL when present", response.Result.ResultSummary)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-run-n-001",
		Source:    asyncRunningSource(),
	}); err != nil {
		t.Fatalf("direct StartAsync: %v", err)
	}
	read, err := service.GetSession(context.Background(), "dur-sess-js-run-n-001")
	if err != nil {
		t.Fatalf("direct GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("direct status = %q, want RUNNING", read.Status)
	}
}

func TestMockClient_GetResult_RunningFixtureReturnsTypedNotReadyEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want not-ready error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want not-ready envelope")
	}
	if response.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("error code = %q, want factory_session.result.not_ready", response.Error.Code)
	}
	if !response.Error.Retryable {
		t.Fatal("retryable = false, want true for running session")
	}
	if response.Error.SessionID != started.Result.SessionId {
		t.Fatalf("sessionId = %q, want %q", response.Error.SessionID, started.Result.SessionId)
	}
	if response.Error.Details == nil || response.Error.Details["reason"] != "RESULT_NOT_READY" {
		t.Fatalf("details = %#v, want RESULT_NOT_READY reason", response.Error.Details)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP async polling test keeps running, not-ready, and completed fixture assertions together on one mock-client seam.
func TestMockClient_AsyncPolling_ObservesCompletedFixtureThroughStatusAndResult(t *testing.T) {
	client := newFixtureMCPClient(t)

	runningStart, err := client.StartAsync(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	if runningStart.Error != nil || runningStart.Result == nil {
		t.Fatalf("running start = %#v, want success", runningStart)
	}

	runningStatus, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: runningStart.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession running: %v", err)
	}
	if runningStatus.Error != nil || runningStatus.Result == nil {
		t.Fatalf("running status = %#v, want success", runningStatus)
	}
	if runningStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("running status = %q, want RUNNING", runningStatus.Result.Status)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: runningStart.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult not-ready: %v", err)
	}
	if notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("not-ready = %#v, want factory_session.result.not_ready", notReady.Error)
	}

	completedStart, err := client.StartSync(syncSuccessExecutionRequest())
	if err != nil {
		t.Fatalf("StartSync completed: %v", err)
	}
	if completedStart.Error != nil || completedStart.Result == nil {
		t.Fatalf("completed start = %#v, want success", completedStart)
	}

	completedStatus, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: completedStart.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession completed: %v", err)
	}
	if completedStatus.Error != nil || completedStatus.Result == nil {
		t.Fatalf("completed status = %#v, want success", completedStatus)
	}
	if completedStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("completed status = %q, want SUCCEEDED", completedStatus.Result.Status)
	}
	if completedStatus.Result.ResultSummary == nil ||
		completedStatus.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", completedStatus.Result.ResultSummary)
	}

	completedResult, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: completedStart.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult completed: %v", err)
	}
	if completedResult.Error != nil || completedResult.Result == nil {
		t.Fatalf("completed result = %#v, want terminal result", completedResult)
	}
	if completedResult.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", completedResult.Result.ResultStatus)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-run-n-001",
		Source:    asyncRunningSource(),
	}); err != nil {
		t.Fatalf("direct StartAsync running: %v", err)
	}
	runningResult, err := service.GetResult(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ResultRequest{Mode: factorysessionexecution.ResultModeFinal},
	)
	if err != nil {
		t.Fatalf("direct GetResult running: %v", err)
	}
	wantRunningHash, err := fixtures.ProjectedResultReadHash(runningResult)
	if err != nil {
		t.Fatalf("fixtures.ProjectedResultReadHash running: %v", err)
	}
	if wantRunningHash != "sha256:5847ff5f39efb7f12c0a8ca67635c7e7d7332b56dbf7d724d4669450b3c8f765" {
		t.Fatalf("running not-ready hash drift = %q", wantRunningHash)
	}
}

func TestMockClient_StartAsync_MalformedRequestReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want request validation envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for malformed start request")
	}
}

func TestMockClient_StartAsync_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.StartAsync(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want unavailable service envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want unavailable service envelope")
	}
	if response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("error code = %q, want factory_session.service.unavailable", response.Error.Code)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP sync-start test keeps terminal session fields and golden-hash assertions together on one mock-client seam.
func TestMockClient_StartSync_SuccessFixtureReturnsTerminalSession(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := syncSuccessExecutionRequest()

	response, err := client.StartSync(request)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want sync execution response")
	}
	if response.Result.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q, want dur-sess-petri-success-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", response.Result.Status)
	}
	if response.Result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", response.Result.SyncOutcome)
	}
	if response.Result.SourceHash == nil || *response.Result.SourceHash == "" {
		t.Fatal("sourceHash missing from sync success response")
	}
	if response.Result.ResolvedSource.SourceHash == nil || *response.Result.ResolvedSource.SourceHash == "" {
		t.Fatal("resolvedSource.sourceHash missing from sync success response")
	}
	if response.Result.Result == nil || response.Result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL result summary", response.Result.Result)
	}
	if response.Result.Links == nil || response.Result.Links.Results == nil {
		t.Fatal("links.results missing from sync success response")
	}

	serviceResult, err := fixtureFakeService(t).StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
		Args: map[string]any{"ticketId": "TKT-2002"},
	})
	if err != nil {
		t.Fatalf("direct StartSync: %v", err)
	}
	wantHash, err := fixtures.SyncStartResultHash(serviceResult)
	if err != nil {
		t.Fatalf("fixtures.SyncStartResultHash: %v", err)
	}
	if wantHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP terminal-result test keeps result payload fields and golden-hash assertions together on one mock-client seam.
func TestMockClient_GetResult_TerminalSessionReturnsDeterministicResult(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := syncSuccessExecutionRequest()

	started, err := client.StartSync(request)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want terminal result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want terminal Factory Session result")
	}
	if response.Result.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q, want dur-sess-petri-success-001", response.Result.SessionId)
	}
	if response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", response.Result.ResultStatus)
	}
	if response.Result.SessionStatus == nil || *response.Result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sessionStatus = %#v, want SUCCEEDED", response.Result.SessionStatus)
	}
	if response.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal result")
	}
	if response.Result.ArtifactIds == nil || len(*response.Result.ArtifactIds) == 0 {
		t.Fatalf("artifactIds = %#v, want related artifact identifiers", response.Result.ArtifactIds)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
		Args: map[string]any{"ticketId": "TKT-2002"},
	}); err != nil {
		t.Fatalf("direct StartSync: %v", err)
	}
	serviceResult, err := service.GetResult(
		context.Background(),
		"dur-sess-petri-success-001",
		factorysessionexecution.ResultRequest{Mode: factorysessionexecution.ResultModeFinal},
	)
	if err != nil {
		t.Fatalf("direct GetResult: %v", err)
	}
	wantHash, err := fixtures.ProjectedResultReadHash(serviceResult)
	if err != nil {
		t.Fatalf("fixtures.ProjectedResultReadHash: %v", err)
	}
	if wantHash != "sha256:977772c884f0ec53b9292ca8fa0374fec1673fec8d0d481e358b3dd4ae65fb95" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

func TestMockClient_StartSync_RepeatedInvocationReturnsStableSessionIdentity(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := syncSuccessExecutionRequest()

	first, err := client.StartSync(request)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	if first.Error != nil || first.Result == nil {
		t.Fatalf("first = %#v, want success", first)
	}

	second, err := client.StartSync(request)
	if err != nil {
		t.Fatalf("second StartSync: %v", err)
	}
	if second.Error != nil || second.Result == nil {
		t.Fatalf("second = %#v, want success", second)
	}
	if second.Result.SessionId != first.Result.SessionId {
		t.Fatalf("sessionId drift: first %q, second %q", first.Result.SessionId, second.Result.SessionId)
	}
	if second.Result.Status != first.Result.Status {
		t.Fatalf("status drift: first %q, second %q", first.Result.Status, second.Result.Status)
	}
	if second.Result.SyncOutcome != first.Result.SyncOutcome {
		t.Fatalf("syncOutcome drift: first %q, second %q", first.Result.SyncOutcome, second.Result.SyncOutcome)
	}
}

func TestMockClient_WorkflowRunCompatibilityOnlyAliasMatchesCanonicalSuccess(t *testing.T) {
	request := syncSuccessExecutionRequest()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	canonicalClient := newFixtureMCPClient(t)
	aliasClient := newFixtureMCPClient(t)
	canonicalRaw, err := canonicalClient.CallTool(mcpfactorysession.ToolStartSync, encoded)
	if err != nil {
		t.Fatalf("canonical start sync: %v", err)
	}
	aliasRaw, err := aliasClient.CallTool(mcpfactorysession.ToolWorkflowRun, encoded)
	if err != nil {
		t.Fatalf("alias start sync: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias response = %s, want canonical %s", aliasRaw, canonicalRaw)
	}

	var response mcpfactorysession.ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]
	if err := json.Unmarshal(aliasRaw, &response); err != nil {
		t.Fatalf("unmarshal alias response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want completed sync success", response)
	}
	if response.Result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", response.Result.SyncOutcome)
	}
}

func TestMockClient_StartSync_MalformedRequestReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.StartSync(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want request validation envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for malformed start request")
	}
}

func TestMockClient_StartSync_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.StartSync(syncSuccessExecutionRequest())
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want unavailable service envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want unavailable service envelope")
	}
	if response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("error code = %q, want factory_session.service.unavailable", response.Error.Code)
	}
}

func asyncRunningExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-run-n-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	}
}

func asyncRunningSource() factorysessionexecution.Source {
	return factorysessionexecution.Source{
		Kind:      workflowsource.KindFactoryID,
		FactoryID: "customer-support-triage",
	}
}

func syncSuccessExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-success-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-2002"},
	}
}

func newFixtureMCPClient(t *testing.T) *mcpfactorysession.Client {
	t.Helper()
	return mcpfactorysession.NewClientWithService(fixtureFakeService(t))
}

func fixtureFakeService(t *testing.T) *factorysessionexecution.FakeService {
	t.Helper()
	path := filepath.Join("..", "..", "http", "testdata", "durable-session-contract-fixtures.json")
	service, err := testharness.New(testharness.Config{
		Mode:            testharness.ModeFake,
		FakeFixturePath: path,
	})
	if err != nil {
		t.Fatalf("compose fixture-backed MCP execution service: %v", err)
	}
	fakeService, ok := service.(*factorysessionexecution.FakeService)
	if !ok {
		t.Fatalf("fixture-backed MCP execution service = %T, want *factorysessionexecution.FakeService", service)
	}
	return fakeService
}

func strPtr(value string) *string {
	return &value
}

func TestMockClient_ListSessions_DefaultsToLiveScope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("list = %#v, want success", response)
	}
	if response.Result.Scope == nil || *response.Result.Scope != factoryapi.FactorySessionListScopeLive {
		t.Fatalf("scope = %#v, want live", response.Result.Scope)
	}
}

func TestMockClient_ListSessions_ScopedPersistedAndAll(t *testing.T) {
	client := newFixtureMCPClient(t)
	seedRunningAndSuccessSessions(t, client)

	persistedScope := factoryapi.FactorySessionListScopePersisted
	persisted, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &persistedScope})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	persistedResult := requireListSessionsSuccess(t, persisted)
	assertListScope(t, persistedResult, factoryapi.FactorySessionListScopePersisted)
	if len(persistedResult.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for persisted scope", persistedResult.Sessions)
	}
	assertContainsDurableSession(t, persistedResult, "dur-sess-petri-success-001")
	assertOmitsDurableSession(t, persistedResult, "dur-sess-petri-run-001")

	allScope := factoryapi.FactorySessionListScopeAll
	all, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &allScope})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	allResult := requireListSessionsSuccess(t, all)
	assertListScope(t, allResult, factoryapi.FactorySessionListScopeAll)
	if len(allResult.Sessions) != 1 || allResult.Sessions[0].Id != "dur-sess-petri-run-001" {
		t.Fatalf("sessions = %#v, want deduped live running row", allResult.Sessions)
	}
	assertContainsDurableSession(t, allResult, "dur-sess-petri-success-001")
}

func seedRunningAndSuccessSessions(t *testing.T, client *mcpfactorysession.Client) {
	t.Helper()
	if _, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-1001"},
	}); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	if _, err := client.StartSync(syncSuccessExecutionRequest()); err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
}

func requireListSessionsSuccess(
	t *testing.T,
	response mcpfactorysession.ToolResponse[factoryapi.ListFactorySessionsResponse],
) *factoryapi.ListFactorySessionsResponse {
	t.Helper()
	if response.Error != nil || response.Result == nil {
		t.Fatalf("list = %#v, want success", response)
	}
	return response.Result
}

func assertListScope(t *testing.T, result *factoryapi.ListFactorySessionsResponse, scope factoryapi.FactorySessionListScope) {
	t.Helper()
	if result.Scope == nil || *result.Scope != scope {
		t.Fatalf("scope = %#v, want %q", result.Scope, scope)
	}
}

func assertContainsDurableSession(t *testing.T, result *factoryapi.ListFactorySessionsResponse, sessionID string) {
	t.Helper()
	if result.DurableSessions == nil {
		t.Fatalf("durableSessions = nil, want row %q", sessionID)
	}
	for _, row := range *result.DurableSessions {
		if row.SessionId == sessionID {
			return
		}
	}
	t.Fatalf("durableSessions = %#v, want row %q", result.DurableSessions, sessionID)
}

func assertOmitsDurableSession(t *testing.T, result *factoryapi.ListFactorySessionsResponse, sessionID string) {
	t.Helper()
	if result.DurableSessions == nil {
		return
	}
	for _, row := range *result.DurableSessions {
		if row.SessionId == sessionID {
			t.Fatalf("durableSessions unexpectedly contain %q", sessionID)
		}
	}
}

func TestMockClient_ListSessions_UnsupportedScopeReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)
	scope := factoryapi.FactorySessionListScope("workspace")
	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &scope})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want validation envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
}

func TestMockClient_ListSessions_UnavailableServiceReturnsStableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()
	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want unavailable service envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want unavailable service envelope")
	}
	if response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("error code = %q, want factory_session.service.unavailable", response.Error.Code)
	}
}

const runtimeBusyLoopWorkflowSource = `// Busy loop fixture for runtime-backed MCP async polling tests.
var spin = 0;
while (true) {
  spin += 1;
}
`

const runtimeSimpleFinalWorkflowSource = `// Simple final-only workflow fixture for runtime-backed MCP async completion tests.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

func TestMockClient_RuntimeService_StartAsyncRunningObservesStatusAndNotReadyResult(t *testing.T) {
	client := newRuntimeMCPClient(t)

	started := assertRuntimeAsyncStartRunning(t, client, runtimeBusyLoopAsyncRequest("req-mcp-runtime-async-running-001"))
	assertRuntimeSessionStatus(t, client, started.Result.SessionId, factoryapi.FactorySessionDurableLifecycleStatusRunning)
	assertRuntimeNotReadyResult(t, client, started.Result.SessionId)
	cancelRuntimeSession(t, client, started.Result.SessionId)
	waitUntilRuntimeSessionStatus(t, runtimeServiceFromClient(t, client), started.Result.SessionId, factorysessionexecution.LifecycleStatusCanceled, 5*time.Second)
}

func TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult(t *testing.T) {
	client := newRuntimeMCPClient(t)
	request := runtimeSimpleFinalAsyncRequest("req-mcp-runtime-async-final-001")

	started := assertRuntimeAsyncStartRunning(t, client, request)
	service := runtimeServiceFromClient(t, client)
	session := waitUntilRuntimeSessionStatus(
		t,
		service,
		started.Result.SessionId,
		factorysessionexecution.LifecycleStatusSucceeded,
		5*time.Second,
	)
	assertRuntimeFinalResultSummary(t, session)
	assertRuntimeTerminalSessionReads(t, client, started.Result.SessionId)
}

func assertRuntimeAsyncStartRunning(
	t *testing.T,
	client *runtimeMCPClient,
	request factoryapi.FactorySessionExecutionRequest,
) mcpfactorysession.ToolResponse[factoryapi.FactorySessionExecutionResponse] {
	t.Helper()
	started, err := client.StartAsync(request)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start status = %q, want RUNNING", started.Result.Status)
	}
	if started.Result.SessionId == "" {
		t.Fatal("sessionId missing from async start response")
	}
	return started
}

func assertRuntimeSessionStatus(
	t *testing.T,
	client *runtimeMCPClient,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	status, err := client.GetSession(mcpfactorysession.GetSessionInput{SessionID: sessionID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if status.Error != nil || status.Result == nil {
		t.Fatalf("status = %#v, want success", status)
	}
	if status.Result.Status != want {
		t.Fatalf("status = %q, want %q", status.Result.Status, want)
	}
}

func assertRuntimeNotReadyResult(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: sessionID,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if notReady.Result != nil {
		t.Fatalf("result = %#v, want not-ready envelope", notReady.Result)
	}
	if notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("error = %#v, want factory_session.result.not_ready", notReady.Error)
	}
	if !notReady.Error.Retryable {
		t.Fatal("retryable = false, want true for running session")
	}
}

func cancelRuntimeSession(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	service := runtimeServiceFromClient(t, client)
	cancelled, err := service.Cancel(context.Background(), sessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", cancelled.Outcome)
	}
}

func assertRuntimeFinalResultSummary(t *testing.T, session factorysessionexecution.SessionReadResult) {
	t.Helper()
	if session.ResultSummary == nil ||
		session.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
}

func assertRuntimeTerminalSessionReads(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	assertRuntimeSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusSucceeded)

	mode := factoryapi.FactorySessionResultModeFinal
	completedResult, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: sessionID,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult completed: %v", err)
	}
	if completedResult.Error != nil || completedResult.Result == nil {
		t.Fatalf("completed result = %#v, want terminal result", completedResult)
	}
	if completedResult.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", completedResult.Result.ResultStatus)
	}
	if completedResult.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal result")
	}
}

func newRuntimeMCPClient(t *testing.T) *runtimeMCPClient {
	t.Helper()
	projectRoot := t.TempDir()
	service, err := testharness.New(testharness.Config{
		Mode:              testharness.ModeJavaScript,
		ProjectRoot:       projectRoot,
		Clock:             platformclock.Real{},
		Persistence:       runtimepersist.DirectoryStore{Dir: filepath.Join(projectRoot, "durable-sessions")},
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeFake,
	})
	if err != nil {
		t.Fatalf("compose runtime-backed MCP execution service: %v", err)
	}
	t.Cleanup(func() {
		drainRuntimeMCPClientSessions(t, service)
		removeRuntimeMCPProjectState(t, projectRoot)
	})
	return &runtimeMCPClient{
		Client:  mcpfactorysession.NewClientWithService(service),
		service: service,
	}
}

type runtimeMCPClient struct {
	*mcpfactorysession.Client
	service factorysessionexecution.Service
}

func runtimeServiceFromClient(t *testing.T, client *runtimeMCPClient) factorysessionexecution.Service {
	t.Helper()
	if client.service == nil {
		t.Fatal("runtime service missing from MCP client wrapper")
	}
	return client.service
}

func runtimeBusyLoopAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, runtimeBusyLoopWorkflowSource, map[string]any{
		"subject": "workflows",
	})
}

func runtimeSimpleFinalAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, runtimeSimpleFinalWorkflowSource, map[string]any{
		"subject": "workflows",
		"count":   2,
		"prefix":  "you",
	})
}

func runtimeInlineAsyncRequest(
	requestID string,
	source string,
	args map[string]any,
) factoryapi.FactorySessionExecutionRequest {
	dialect := "you-workflow-v1"
	metadata := factoryapi.StringMap{
		"name":        "runtime-mcp-async-fixture",
		"description": "runtime-backed MCP async polling fixture",
	}
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   source,
				},
				Dialect:  &dialect,
				Metadata: &metadata,
			},
		},
		Args: &args,
	}
}

func waitUntilRuntimeSessionStatus(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	want factorysessionexecution.LifecycleStatus,
	timeout time.Duration,
) factorysessionexecution.SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if session.Status == want {
			return session
		}
		if factorysessionexecution.IsTerminalLifecycleStatus(session.Status) && session.Status != want {
			t.Fatalf("session %s reached terminal %q before %q", sessionID, session.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %q within %s", sessionID, want, timeout)
	return factorysessionexecution.SessionReadResult{}
}
