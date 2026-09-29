package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
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

type subagentTargetFake struct {
	factorysessions.TargetExecutionService
	start        factorysessions.StartRequest
	invoke       factorysessions.InvocationRequest
	started      bool
	closed       bool
	invokeErr    error
	invokeResult *factorysessions.InvocationResult
	wait         <-chan struct{}
}

func (fake *subagentTargetFake) StartAsync(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	fake.start = request
	fake.started = true
	return factorysessions.AsyncStartResult{SessionID: "session-1", Status: "RUNNING"}, nil
}

func (fake *subagentTargetFake) InvokeFactorySession(ctx context.Context, _ string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	fake.invoke = request
	if fake.wait != nil {
		select {
		case <-fake.wait:
		case <-ctx.Done():
			return factorysessions.InvocationResult{}, ctx.Err()
		}
	}
	if fake.invokeResult != nil {
		return *fake.invokeResult, fake.invokeErr
	}
	if fake.invokeErr != nil {
		return factorysessions.InvocationResult{}, fake.invokeErr
	}
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "subagent answer"}},
	}, nil
}

func TestSubagentWaitsForLongInvocationAndForwardsOneHourTimeout(t *testing.T) {
	release := make(chan struct{})
	target := &subagentTargetFake{wait: release}
	timeoutMillis := int64(3_600_000)
	finished := make(chan mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], 1)
	go func() {
		finished <- mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-long" }, mcpfactorysession.SubagentInput{Prompt: "Wait for the result", TimeoutMillis: &timeoutMillis})
	}()
	select {
	case response := <-finished:
		t.Fatalf("subagent returned before provider completion: %#v", response)
	case <-time.After(1500 * time.Millisecond):
	}
	close(release)
	select {
	case response := <-finished:
		if response.Error != nil || response.Result == nil || response.Result.Status != string(factorysessions.InvocationTerminalStatusCompleted) {
			t.Fatalf("subagent response = %#v", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subagent did not return after provider completion")
	}
	if target.invoke.TimeoutMillis == nil || *target.invoke.TimeoutMillis != timeoutMillis || !target.closed {
		t.Fatalf("invocation timeout = %#v, closed = %t", target.invoke.TimeoutMillis, target.closed)
	}
}

func (fake *subagentTargetFake) CloseFactorySession(_ context.Context, sessionID string) error {
	fake.closed = sessionID == "session-1"
	return nil
}

func TestSubagentRunsPackagedFactoryWithDefaultsAndReturnsText(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-1" }, mcpfactorysession.SubagentInput{Prompt: "Summarize this"})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Result.Text != "subagent answer" || response.Result.SessionID != "session-1" {
		t.Fatalf("Subagent result = %#v", response.Result)
	}
	if target.start.Source.Kind != factoryruntime.WorkflowSourceKindFactoryID || target.start.Source.FactoryID != factorydefinitions.PackagedSubagentFactoryName {
		t.Fatalf("start source = %#v", target.start.Source)
	}
	if target.start.Args["workingRoot"] != "C:/project" {
		t.Fatalf("workingRoot = %#v", target.start.Args)
	}
	if got := *target.invoke.Args; len(got) != 1 || got["input"] != "Summarize this" {
		t.Fatalf("default invocation args = %#v", got)
	}
	if !target.started || !target.closed {
		t.Fatalf("lifecycle start=%t close=%t", target.started, target.closed)
	}
}

func TestSubagentForwardsModelOverridesAndCleansUpOnInvocationFailure(t *testing.T) {
	target := &subagentTargetFake{invokeErr: errors.New("provider unavailable")}
	response := mcpfactorysession.Subagent(context.Background(), target, "", func() string { return "request-2" }, mcpfactorysession.SubagentInput{
		Prompt: "Explain this", Provider: "opencode", Model: "local-model", ReasoningEffort: "high",
	})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	args := *target.invoke.Args
	if args["workerProvider"] != "opencode" || args["workerModel"] != "local-model" || args["workerReasoningEffort"] != "high" {
		t.Fatalf("override args = %#v", args)
	}
	if !target.closed {
		t.Fatal("Factory Session was not closed after invocation failure")
	}
}

func TestSubagentTimeoutReportsPossibleWorkspaceEffects(t *testing.T) {
	timeout := int64(180000)
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:    factorysessions.InvocationTerminalStatusTimedOut,
		ErrorCode: string(factorysessions.InvocationErrorCodeTimedOut),
		Message:   "sensitive provider transcript",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, mcpfactorysession.SubagentInput{
		Prompt: "Edit a file", TimeoutMillis: &timeout,
	})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable || response.Error.SessionID != "session-1" {
		t.Fatalf("timeout envelope = %#v", response.Error)
	}
	if response.Error.Details["partialEffectsPossible"] != true || response.Error.Details["timeoutMillis"] != timeout || response.Error.Details["invocationCode"] != string(factorysessions.InvocationErrorCodeTimedOut) {
		t.Fatalf("timeout details = %#v", response.Error.Details)
	}
	if strings.Contains(response.Error.Message, "sensitive") || !target.closed {
		t.Fatalf("timeout leaked provider text or skipped close: %#v", response.Error)
	}
}

func TestSubagentTimeoutResultWithInvocationErrorKeepsTimeoutEnvelope(t *testing.T) {
	timeout := int64(5000)
	target := &subagentTargetFake{
		invokeErr: context.DeadlineExceeded,
		invokeResult: &factorysessions.InvocationResult{
			Status:    factorysessions.InvocationTerminalStatusTimedOut,
			ErrorCode: string(factorysessions.InvocationErrorCodeTimedOut),
			Message:   "private provider output",
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout-error" }, mcpfactorysession.SubagentInput{
		Prompt: "Edit a file", TimeoutMillis: &timeout,
	})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.timed_out" || response.Error.SessionID != "session-1" || response.Error.Retryable {
		t.Fatalf("timeout envelope = %#v", response.Error)
	}
	if response.Error.Details["partialEffectsPossible"] != true || response.Error.Details["timeoutMillis"] != timeout {
		t.Fatalf("timeout details = %#v", response.Error.Details)
	}
	if strings.Contains(response.Error.Message, "private") || !target.closed {
		t.Fatalf("timeout leaked provider text or skipped close: %#v", response.Error)
	}
}
