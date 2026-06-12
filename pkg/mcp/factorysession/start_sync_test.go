package factorysession_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

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

func TestMockClient_WorkflowRunAliasMatchesCanonicalSuccess(t *testing.T) {
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
	path := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(path)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func strPtr(value string) *string {
	return &value
}
