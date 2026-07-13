package factorysession_test

import (
	"context"
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestMockClient_GetSession_FailedFixtureReturnsDeterministicStatusWithPartialSummary(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(failedPartialExecutionRequest())
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
		t.Fatalf("read = %#v, want failed session read model", response)
	}
	if response.Result.SessionId != "dur-sess-js-failed-partial-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-failed-partial-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("status = %q, want FAILED", response.Result.Status)
	}
	if response.Result.ResultSummary == nil ||
		response.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFailedWithPartial {
		t.Fatalf("resultSummary = %#v, want FAILED_WITH_PARTIAL", response.Result.ResultSummary)
	}
	if response.Result.FailureDetail == nil ||
		response.Result.PartialResultAvailable == nil ||
		!*response.Result.PartialResultAvailable {
		t.Fatalf("failure = %#v, want partialResultAvailable=true", response.Result.FailureDetail)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP failure-path test keeps partial-result and golden-hash assertions together on one mock-client seam.
func TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(failedPartialExecutionRequest())
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	mode := factoryapi.FactorySessionResultModePartial
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("result = %#v, want FAILED_WITH_PARTIAL partial result", response)
	}
	if response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFailedWithPartial {
		t.Fatalf("resultStatus = %q, want FAILED_WITH_PARTIAL", response.Result.ResultStatus)
	}
	if response.Result.SessionStatus == nil ||
		*response.Result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("sessionStatus = %#v, want FAILED", response.Result.SessionStatus)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) == 0 {
		t.Fatal("primaryResult missing from failed-with-partial result")
	}
	if response.Result.FailureDetail == nil ||
		response.Result.PartialResultAvailable == nil ||
		!*response.Result.PartialResultAvailable {
		t.Fatalf("failure = %#v, want partialResultAvailable=true", response.Result.FailureDetail)
	}

	service := fixtureFakeService(t)
	if _, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-failed-partial-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}); err != nil {
		t.Fatalf("direct StartAsync: %v", err)
	}
	serviceResult, err := service.GetResult(
		context.Background(),
		"dur-sess-js-failed-partial-001",
		factorysessionexecution.ResultRequest{Mode: factorysessionexecution.ResultModePartial},
	)
	if err != nil {
		t.Fatalf("direct GetResult: %v", err)
	}
	wantHash, err := fixtures.ProjectedResultReadHash(serviceResult)
	if err != nil {
		t.Fatalf("fixtures.ProjectedResultReadHash: %v", err)
	}
	if wantHash != "sha256:36530c7711e80e60d05ab82c87a6cebbf9db4966b028dfdcbf0b9008d1d47a68" {
		t.Fatalf("golden hash drift = %q", wantHash)
	}
}

func TestMockClient_GetSession_UnknownSessionReturnsTypedNotFoundEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: "dur-sess-missing-999",
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want not-found error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want not-found envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
	if response.Error.SessionID != "dur-sess-missing-999" {
		t.Fatalf("sessionId = %q, want dur-sess-missing-999", response.Error.SessionID)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for missing session")
	}
}

func TestMockClient_GetResult_UnknownSessionReturnsTypedNotFoundEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: "dur-sess-missing-999",
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want not-found error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want not-found envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
	if response.Error.SessionID != "dur-sess-missing-999" {
		t.Fatalf("sessionId = %q, want dur-sess-missing-999", response.Error.SessionID)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for missing session")
	}
}

func TestMockClient_WorkflowStatusCompatibilityOnlyAlias_MissingSessionMatchesCanonicalNotFound(t *testing.T) {
	input := mcpfactorysession.GetSessionInput{SessionID: "dur-sess-missing-999"}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	canonicalClient := newFixtureMCPClient(t)
	aliasClient := newFixtureMCPClient(t)
	canonicalRaw, err := canonicalClient.CallTool(mcpfactorysession.ToolGetSession, encoded)
	if err != nil {
		t.Fatalf("canonical get session: %v", err)
	}
	aliasRaw, err := aliasClient.CallTool(mcpfactorysession.ToolWorkflowStatus, encoded)
	if err != nil {
		t.Fatalf("alias get session: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias response = %s, want canonical %s", aliasRaw, canonicalRaw)
	}
}

func TestMockClient_WorkflowResultCompatibilityOnlyAliasMatchesCanonicalTerminalResult(t *testing.T) {
	canonicalClient := newFixtureMCPClient(t)
	aliasClient := newFixtureMCPClient(t)
	request := syncSuccessExecutionRequest()
	var sessionID string
	for _, client := range []*mcpfactorysession.Client{canonicalClient, aliasClient} {
		started, err := client.StartSync(request)
		if err != nil {
			t.Fatalf("StartSync: %v", err)
		}
		if started.Error != nil || started.Result == nil {
			t.Fatalf("start = %#v, want success", started)
		}
		if sessionID == "" {
			sessionID = started.Result.SessionId
		} else if started.Result.SessionId != sessionID {
			t.Fatalf("sessionId = %q, want %q", started.Result.SessionId, sessionID)
		}
	}

	mode := factoryapi.FactorySessionResultModeFinal
	encoded, err := json.Marshal(mcpfactorysession.GetResultInput{
		SessionID: sessionID,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	canonicalRaw, err := canonicalClient.CallTool(mcpfactorysession.ToolGetResult, encoded)
	if err != nil {
		t.Fatalf("canonical get result: %v", err)
	}
	aliasRaw, err := aliasClient.CallTool(mcpfactorysession.ToolWorkflowResult, encoded)
	if err != nil {
		t.Fatalf("alias get result: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias response = %s, want canonical %s", aliasRaw, canonicalRaw)
	}
}

func TestMockClient_StartAsync_RequestIDConflictReturnsTypedEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := failedPartialExecutionRequest()

	first, err := client.StartAsync(request)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	if first.Error != nil || first.Result == nil {
		t.Fatalf("first = %#v, want success", first)
	}

	conflict := request
	conflict.Args = &map[string]any{"task": "different"}
	response, err := client.StartAsync(conflict)
	if err != nil {
		t.Fatalf("conflict StartAsync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want request-conflict error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want request-conflict envelope")
	}
	if response.Error.Code != "factory_session.start.request_id_conflict" {
		t.Fatalf("error code = %q, want factory_session.start.request_id_conflict", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for request conflict")
	}
}

func TestMockClient_StartSync_RequestIDConflictReturnsTypedEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)
	request := syncSuccessExecutionRequest()

	first, err := client.StartSync(request)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	if first.Error != nil || first.Result == nil {
		t.Fatalf("first = %#v, want success", first)
	}

	conflict := request
	conflict.Args = &map[string]any{"ticketId": "TKT-CONFLICT"}
	response, err := client.StartSync(conflict)
	if err != nil {
		t.Fatalf("conflict StartSync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want request-conflict error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want request-conflict envelope")
	}
	if response.Error.Code != "factory_session.start.request_id_conflict" {
		t.Fatalf("error code = %q, want factory_session.start.request_id_conflict", response.Error.Code)
	}
}

func failedPartialExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-failed-partial-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	}
}
