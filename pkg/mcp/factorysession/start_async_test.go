package factorysession_test

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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
