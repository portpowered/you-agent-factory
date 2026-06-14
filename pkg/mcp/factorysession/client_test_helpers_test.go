package factorysession_test

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
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
