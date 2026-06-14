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

func petriSuccessSyncRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-success-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-2002"},
	}
}

func petriSuccessStartRequest() factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
		Args: map[string]any{"ticketId": "TKT-2002"},
	}
}

func requireSyncStartSuccess(
	t *testing.T,
	response mcpfactorysession.ToolResponse[factoryapi.FactorySessionSyncExecutionResponse],
	err error,
) *factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("start = %#v, want success", response)
	}
	return response.Result
}

func assertSyncSuccessResponse(t *testing.T, result *factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q, want dur-sess-petri-success-001", result.SessionId)
	}
	if result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", result.Status)
	}
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", result.SyncOutcome)
	}
	if result.SourceHash == nil || *result.SourceHash == "" {
		t.Fatal("sourceHash missing from sync success response")
	}
	if result.ResolvedSource.SourceHash == nil || *result.ResolvedSource.SourceHash == "" {
		t.Fatal("resolvedSource.sourceHash missing from sync success response")
	}
	if result.Result == nil || result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL result summary", result.Result)
	}
	if result.Links == nil || result.Links.Results == nil {
		t.Fatal("links.results missing from sync success response")
	}
}

func assertTerminalGetResult(t *testing.T, response mcpfactorysession.ToolResponse[factoryapi.FactorySessionResult]) {
	t.Helper()
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
}

func assertDispatchListSummary(t *testing.T, response *factoryapi.ListFactorySessionDispatchesResponse) {
	t.Helper()
	if response.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q, want dur-sess-petri-success-001", response.SessionId)
	}
	if len(response.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one summary", response.Dispatches)
	}
	dispatch := response.Dispatches[0]
	if dispatch.Id != "disp-petri-success-001" {
		t.Fatalf("dispatchId = %q, want disp-petri-success-001", dispatch.Id)
	}
	if dispatch.Status == "" || dispatch.DispatchKind == "" {
		t.Fatalf("dispatch missing status/kind: %#v", dispatch)
	}
	if dispatch.Label == nil || *dispatch.Label == "" {
		t.Fatal("label missing from dispatch summary")
	}
}

func assertArtifactListSummary(t *testing.T, response *factoryapi.ListFactorySessionArtifactsResponse) {
	t.Helper()
	if response.SessionId != "dur-sess-js-paused-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-paused-001", response.SessionId)
	}
	if len(response.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one summary", response.Artifacts)
	}
	artifact := response.Artifacts[0]
	if artifact.Id != "art-js-pause-001" {
		t.Fatalf("artifactId = %q, want art-js-pause-001", artifact.Id)
	}
	if artifact.Kind == "" {
		t.Fatalf("kind missing from artifact summary: %#v", artifact)
	}
	if artifact.Visibility == "" {
		t.Fatalf("visibility missing from artifact summary: %#v", artifact)
	}
	if artifact.ContentHash == nil || *artifact.ContentHash == "" {
		t.Fatalf("contentHash missing from artifact summary: %#v", artifact)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes <= 0 {
		t.Fatalf("sizeBytes missing from artifact summary: %#v", artifact)
	}
	if artifact.DispatchId == nil || *artifact.DispatchId != "disp-js-pause-001" {
		t.Fatalf("dispatchId = %#v, want disp-js-pause-001", artifact.DispatchId)
	}
	if artifact.RetrievalRef == nil || artifact.RetrievalRef.Href == "" {
		t.Fatalf("retrievalRef missing from artifact summary: %#v", artifact)
	}
}

func seedPetriSuccessOnService(t *testing.T, service *factorysessionexecution.FakeService) {
	t.Helper()
	if _, err := service.StartSync(context.Background(), petriSuccessStartRequest()); err != nil {
		t.Fatalf("direct StartSync: %v", err)
	}
}

func requireGoldenHash(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("golden hash drift = %q, want %q", got, want)
	}
}

func TestMockClient_GetResult_TerminalSessionReturnsDeterministicResult(t *testing.T) {
	client := newFixtureMCPClient(t)

	startResponse, startErr := client.StartSync(petriSuccessSyncRequest())
	started := requireSyncStartSuccess(t, startResponse, startErr)

	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	assertTerminalGetResult(t, response)

	service := fixtureFakeService(t)
	seedPetriSuccessOnService(t, service)
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
	requireGoldenHash(t, wantHash, "sha256:977772c884f0ec53b9292ca8fa0374fec1673fec8d0d481e358b3dd4ae65fb95")
}

func TestMockClient_ListDispatches_DispatchInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := newFixtureMCPClient(t)

	startResponse, startErr := client.StartSync(petriSuccessSyncRequest())
	started := requireSyncStartSuccess(t, startResponse, startErr)

	response, err := client.ListDispatches(mcpfactorysession.ListDispatchesInput{
		SessionID: started.SessionId,
	})
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want dispatch list", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want dispatch list response")
	}
	assertDispatchListSummary(t, response.Result)

	service := fixtureFakeService(t)
	seedPetriSuccessOnService(t, service)
	serviceListed, err := service.ListDispatches(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("direct ListDispatches: %v", err)
	}
	wantHash, err := fixtures.ListDispatchesResultHash(serviceListed)
	if err != nil {
		t.Fatalf("fixtures.ListDispatchesResultHash: %v", err)
	}
	requireGoldenHash(t, wantHash, "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a")
}

func TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-paused-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: strPtr(".claude/workflows/approval-gate.yaml"),
		},
		Args: &map[string]any{"changeId": "CHG-42"},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	response, err := client.ListArtifacts(mcpfactorysession.ListArtifactsInput{
		SessionID: started.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want artifact list", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want artifact list response")
	}
	assertArtifactListSummary(t, response.Result)

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-paused-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
		Args: map[string]any{"changeId": "CHG-42"},
	}); err != nil {
		t.Fatalf("direct StartAsync: %v", err)
	}
	serviceListed, err := service.ListArtifacts(context.Background(), "dur-sess-js-paused-001")
	if err != nil {
		t.Fatalf("direct ListArtifacts: %v", err)
	}
	wantHash, err := fixtures.ListArtifactsResultHash(serviceListed)
	if err != nil {
		t.Fatalf("fixtures.ListArtifactsResultHash: %v", err)
	}
	requireGoldenHash(t, wantHash, "sha256:57fa7af131ce29cb2a254d2548ef8b8f9b0ccf6de7fb6cc185beabf8190f1dcb")
}

func TestMockClient_ListDispatches_MissingSessionReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.ListDispatches(mcpfactorysession.ListDispatchesInput{
		SessionID: "dur-sess-does-not-exist",
	})
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want missing-session envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want missing-session envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
	if response.Error.SessionID != "dur-sess-does-not-exist" {
		t.Fatalf("sessionId = %q, want dur-sess-does-not-exist", response.Error.SessionID)
	}
}

func TestMockClient_ListArtifacts_MissingSessionReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.ListArtifacts(mcpfactorysession.ListArtifactsInput{
		SessionID: "dur-sess-does-not-exist",
	})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want missing-session envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want missing-session envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
}

func TestMockClient_ListDispatches_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.ListDispatches(mcpfactorysession.ListDispatchesInput{
		SessionID: "dur-sess-petri-success-001",
	})
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
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

func TestMockClient_ListArtifacts_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.ListArtifacts(mcpfactorysession.ListArtifactsInput{
		SessionID: "dur-sess-js-paused-001",
	})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
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

func TestMockClient_ListDispatches_RunningFixtureIncludesProviderSessionRefs(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	response, err := client.ListDispatches(mcpfactorysession.ListDispatchesInput{
		SessionID: started.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, want dispatch list", response)
	}
	if len(response.Result.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one running dispatch", response.Result.Dispatches)
	}
	refs := response.Result.Dispatches[0].ProviderSessionRefs
	if refs == nil || len(*refs) != 1 || (*refs)[0].Id != "prov-sess-disp-petri-001" {
		t.Fatalf("providerSessionRefs = %#v, want prov-sess-disp-petri-001", refs)
	}
}

func TestMockClient_ListReadTools_SharedEnvelopeShape(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartSync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-success-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-2002"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{
			name: "get_result",
			raw:  []byte(`{"sessionId":"dur-sess-petri-success-001"}`),
		},
		{
			name: "list_dispatches",
			raw:  []byte(`{"sessionId":"dur-sess-petri-success-001"}`),
		},
		{
			name: "list_artifacts",
			raw:  []byte(`{"sessionId":"dur-sess-petri-success-001"}`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var toolName string
			switch tc.name {
			case "get_result":
				toolName = mcpfactorysession.ToolGetResult
			case "list_dispatches":
				toolName = mcpfactorysession.ToolListDispatches
			case "list_artifacts":
				toolName = mcpfactorysession.ToolListArtifacts
			}
			payload, err := client.CallTool(toolName, tc.raw)
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			var envelope struct {
				Result json.RawMessage                      `json:"result"`
				Error  *mcpfactorysession.ToolErrorEnvelope `json:"error"`
			}
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Error != nil {
				t.Fatalf("error = %#v, want success envelope branch", envelope.Error)
			}
			if len(envelope.Result) == 0 {
				t.Fatal("result branch missing from shared tool response envelope")
			}
		})
	}
}
