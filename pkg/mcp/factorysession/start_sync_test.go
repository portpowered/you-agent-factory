package factorysession_test

import (
	"context"
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

	response, err := client.StartSync(factoryapi.FactorySessionExecutionRequest{
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

func TestMockClient_StartSync_ProviderErrorFixtureReturnsFailedSession(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.StartSync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-missing-source-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("does-not-exist"),
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want failed session result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want failed sync execution response")
	}
	if response.Result.SessionId != "dur-sess-missing-source-001" {
		t.Fatalf("sessionId = %q, want dur-sess-missing-source-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("status = %q, want FAILED", response.Result.Status)
	}
	if response.Result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED for terminal failed session", response.Result.SyncOutcome)
	}
	if response.Result.Result == nil {
		t.Fatal("result summary missing for provider-error fixture")
	}
	if response.Result.Result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("resultStatus = %q, want UNAVAILABLE", response.Result.Result.ResultStatus)
	}
	if response.Result.Result.Failure == nil || response.Result.Result.Failure.Reason == nil {
		t.Fatalf("failure = %#v, want MISSING_SOURCE provider failure", response.Result.Result.Failure)
	}
	if *response.Result.Result.Failure.Reason != "MISSING_SOURCE" {
		t.Fatalf("failure.reason = %q, want MISSING_SOURCE", *response.Result.Result.Failure.Reason)
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

	response, err := client.StartSync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-success-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
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
