package factorysession_test

import (
	"context"
	"encoding/json"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

const failedPartialSessionID = "dur-sess-js-failed-partial-001"

func TestMockClient_GetSession_FailedFixtureReturnsDeterministicStatusWithPartialSummary(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return failedPartialStart(), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return failedPartialSession(), nil
		},
	})
	started, err := client.StartAsync(context.Background(), failedPartialExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, %v; want success", started, err)
	}
	response, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: started.Result.SessionId})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("read = %#v, %v; want failed session", response, err)
	}
	if response.Result.SessionId != failedPartialSessionID ||
		response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session = %#v, want failed session %q", response.Result, failedPartialSessionID)
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

func TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return failedPartialStart(), nil
		},
		getResult: func(_ context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			if sessionID != failedPartialSessionID || request.Mode != factorysessions.ResultModePartial {
				t.Fatalf("GetResult(%q, %#v), want partial failure result", sessionID, request)
			}
			return failedPartialResult(), nil
		},
	})
	started, err := client.StartAsync(context.Background(), failedPartialExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, %v; want success", started, err)
	}
	mode := factoryapi.FactorySessionResultModePartial
	response, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: started.Result.SessionId, Mode: &mode})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("result = %#v, %v; want FAILED_WITH_PARTIAL", response, err)
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
}

func TestMockClient_GetSession_UnknownSessionReturnsTypedNotFoundEnvelope(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
		},
	})
	response, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: "dur-sess-missing-999"})
	assertMissingSessionEnvelope(t, response.Result != nil, response.Error, err)
}

func TestMockClient_GetResult_UnknownSessionReturnsTypedNotFoundEnvelope(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return factorysessions.ResultReadResult{}, factorysessions.ErrDurableSessionNotFound
		},
	})
	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{
		SessionID: "dur-sess-missing-999",
		Mode:      &mode,
	})
	assertMissingSessionEnvelope(t, response.Result != nil, response.Error, err)
}

func TestMockClient_StartAsync_RequestIDConflictReturnsTypedEnvelope(t *testing.T) {
	call := 0
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			call++
			if call == 1 {
				return failedPartialStart(), nil
			}
			return factorysessions.AsyncStartResult{}, factorysessions.ErrExecutionRequestIDConflict
		},
	})
	request := failedPartialExecutionRequest()
	first, err := client.StartAsync(context.Background(), request)
	if err != nil || first.Error != nil || first.Result == nil {
		t.Fatalf("first = %#v, %v; want success", first, err)
	}
	request.Args = &map[string]any{"task": "different"}
	response, err := client.StartAsync(context.Background(), request)
	if err != nil || response.Result != nil || response.Error == nil ||
		response.Error.Code != "factory_session.start.request_id_conflict" || response.Error.Retryable {
		t.Fatalf("conflict = %#v, %v; want non-retryable conflict", response, err)
	}
}

func TestMockClient_StartSync_RequestIDConflictReturnsTypedEnvelope(t *testing.T) {
	call := 0
	client := clientWithScript(scriptedExecutionService{
		startSync: func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			call++
			if call == 1 {
				return successfulSyncStart(), nil
			}
			return factorysessions.SyncStartResult{}, factorysessions.ErrExecutionRequestIDConflict
		},
	})
	request := syncSuccessExecutionRequest()
	first, err := client.StartSync(context.Background(), request)
	if err != nil || first.Error != nil || first.Result == nil {
		t.Fatalf("first = %#v, %v; want success", first, err)
	}
	request.Args = &map[string]any{"ticketId": "TKT-CONFLICT"}
	response, err := client.StartSync(context.Background(), request)
	if err != nil || response.Result != nil || response.Error == nil ||
		response.Error.Code != "factory_session.start.request_id_conflict" {
		t.Fatalf("conflict = %#v, %v; want request ID conflict", response, err)
	}
}

func failedPartialStart() factorysessions.AsyncStartResult {
	return factorysessions.AsyncStartResult{
		SessionID: failedPartialSessionID,
		Status:    string(factorysessions.LifecycleStatusFailed),
		Links:     factorysessions.InspectionLinks{Session: "/factory-sessions/" + failedPartialSessionID},
	}
}

func failedPartialSession() factorysessions.SessionReadResult {
	return factorysessions.SessionReadResult{
		SessionID: failedPartialSessionID,
		Status:    factorysessions.LifecycleStatusFailed,
		ResultSummary: &factorysessions.ResultSummary{
			ResultStatus: "FAILED_WITH_PARTIAL",
		},
		Failure: &factorysessions.FailureSummary{
			Reason:                 "WORKFLOW_FAILED",
			Message:                "workflow failed after partial output",
			PartialResultAvailable: true,
		},
		Usage: factorysessions.EmptySessionUsage(),
	}
}

func failedPartialResult() factorysessions.ResultReadResult {
	return factorysessions.ResultReadResult{
		SessionID:     failedPartialSessionID,
		ResultStatus:  factorysessions.ResultStatus("FAILED_WITH_PARTIAL"),
		SessionStatus: factorysessions.LifecycleStatusFailed,
		Mode:          factorysessions.ResultModePartial,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"partial result"}]`),
		Failure: &factorysessions.FailureSummary{
			Reason:                 "WORKFLOW_FAILED",
			Message:                "workflow failed after partial output",
			PartialResultAvailable: true,
		},
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

func assertMissingSessionEnvelope(
	t *testing.T,
	resultPresent bool,
	envelope *mcpfactorysession.ToolErrorEnvelope,
	err error,
) {
	t.Helper()
	if err != nil || resultPresent || envelope == nil {
		t.Fatalf("response resultPresent=%v error=%#v callErr=%v, want missing-session envelope", resultPresent, envelope, err)
	}
	if envelope.Code != "factory_session.session.not_found" ||
		envelope.SessionID != "dur-sess-missing-999" ||
		envelope.Retryable {
		t.Fatalf("error = %#v, want non-retryable missing-session envelope", envelope)
	}
}
