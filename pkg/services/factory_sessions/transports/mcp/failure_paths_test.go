package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

func TestSubagentTerminalFailureClassification(t *testing.T) {
	const secret = "private-provider-token-123"
	tests := []struct {
		name       string
		reason     string
		code       string
		retryable  bool
		actionText string
	}{
		{
			name: "permanent bad request", reason: string(workers.WorkFailureTypePermanentBadRequest),
			code: "factory_session.subagent.provider_request_rejected", actionText: "selected model against the provider's advertised models and request settings",
		},
		{
			name: "internal server error", reason: string(workers.WorkFailureTypeInternalServerError),
			code: "factory_session.subagent.provider_internal_error", retryable: true, actionText: "Check provider status and logs",
		},
		{
			name: "unknown", reason: string(workers.WorkFailureTypeUnknown),
			code: "factory_session.subagent.provider_unknown_failure", actionText: "Check provider logs and configuration",
		},
		{
			name: "empty reason", code: "factory_session.subagent.execution_failed",
			actionText: "Check provider logs and configuration",
		},
		{
			name: "unrecognized reason", reason: "unrecognized-" + secret,
			code: "factory_session.subagent.execution_failed", actionText: "Inspect the workspace for partial edits",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSubagentTerminalFailureClassification(t, tt.reason, tt.code, tt.retryable, tt.actionText, secret)
		})
	}
}

func assertSubagentTerminalFailureClassification(t *testing.T, reason, code string, retryable bool, actionText, secret string) {
	t.Helper()
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive output " + secret,
		FailureReason: reason,
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-classification" }, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("response = %#v", response)
	}
	if got := response.Error; got.Code != code || got.Retryable != retryable {
		t.Fatalf("code = %q, retryable = %t; want %q, %t", got.Code, got.Retryable, code, retryable)
	}
	if action, ok := response.Error.Details["suggestedAction"].(string); !ok || !strings.Contains(action, actionText) {
		t.Fatalf("suggestedAction = %#v, want text %q", response.Error.Details["suggestedAction"], actionText)
	}
	assertSubagentFailureReason(t, response.Error.Details, reason, secret)
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("response leaked provider text: %s", encoded)
	}
	if !target.closed || target.control.Operation != factorysessions.SessionControlClose || response.Error.Details["sessionClosed"] != true {
		t.Fatalf("session cleanup missing: closed=%t, control=%#v, details=%#v", target.closed, target.control, response.Error.Details)
	}
}

func assertSubagentFailureReason(t *testing.T, details map[string]any, reason, secret string) {
	t.Helper()
	if reason == "" || strings.Contains(reason, secret) {
		if _, ok := details["failureReason"]; ok {
			t.Fatalf("unexpected failureReason = %#v", details["failureReason"])
		}
	} else if got := details["failureReason"]; got != reason {
		t.Fatalf("failureReason = %#v, want %q", got, reason)
	}
}

// The legacy subagent fake embeds Service for methods outside each test. Its
// response-event read is unavailable unless an activity test overrides it.
func (*subagentTargetFake) SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return nil, nil
}

type subagentActivityTarget struct {
	*subagentTargetFake
	events []factorysessions.FactoryResponseEvent
	err    error
	read   bool
}

func (target *subagentActivityTarget) SubscribeFactoryResponseEvents(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	if !target.getBeforeClose || target.closed || request.SessionID != "session-1" || ctx.Err() != nil {
		return nil, errors.New("activity read was not made before close")
	}
	target.read = true
	if target.err != nil {
		return nil, target.err
	}
	return &factorysessions.ResponseEventCursor{
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return target.events, nil },
		DetachCursor: func() {},
	}, nil
}

func TestSubagentTimeoutReportsOnlyBoundedProviderActivity(t *testing.T) {
	const secret = "private provider text and metadata"
	now := time.Now()
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindMessage, Phase: workers.PhaseDelta, RecordedAt: now.Add(-time.Second), Provenance: workers.Provenance{Provider: secret}, ProviderSessionRef: secret, Payload: json.RawMessage(`{"text":"private provider text and metadata"}`)},
			{Kind: workers.Kind(secret), Phase: workers.PhaseUpdated, RecordedAt: now, Provenance: workers.Provenance{Provider: secret}},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-request" }, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || !target.read {
		t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
	}
	progress := response.Error.Details["progress"].(map[string]any)
	activity := progress["lastObservedProviderActivity"].(map[string]any)
	if activity["kind"] != "MESSAGE" || activity["phase"] != "DELTA" || activity["providerSessionObserved"] != true {
		t.Fatalf("last provider activity = %#v", activity)
	}
	observedAt, ok := activity["observedAt"].(time.Time)
	if !ok || !observedAt.Equal(now.Add(-time.Second)) {
		t.Fatalf("observedAt = %#v", activity["observedAt"])
	}
	if len(activity) != 4 {
		t.Fatalf("bounded provider activity = %#v", activity)
	}
	encoded, err := json.Marshal(response.Error)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("timeout leaked provider data: %s, %v", encoded, err)
	}
}

func TestSubagentTimeoutActivityReadFailureStillCloses(t *testing.T) {
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		err:                errors.New("private activity read failure"),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-failure" }, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || !target.closed || !target.read {
		t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
	}
	progress := response.Error.Details["progress"].(map[string]any)
	if _, exists := progress["lastObservedProviderActivity"]; exists || strings.Contains(response.Error.Message, "private activity") {
		t.Fatalf("failed activity read leaked: %#v", response.Error)
	}
	encoded, err := json.Marshal(response.Error)
	if err != nil || strings.Contains(string(encoded), "private activity") {
		t.Fatalf("failed activity read leaked provider data: %s, %v", encoded, err)
	}
}

func TestSubagentTimeoutWithNoProviderEventsOmitsActivity(t *testing.T) {
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-empty" }, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || !target.closed {
		t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
	}
	progress := response.Error.Details["progress"].(map[string]any)
	if activity, exists := progress["lastObservedProviderActivity"]; exists {
		t.Fatalf("unexpected provider activity = %#v", activity)
	}
}

func TestSubagentTimeoutWithResponseEventButNoSessionRefOmitsObservation(t *testing.T) {
	now := time.Now()
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindMessage, Phase: workers.PhaseDelta, RecordedAt: now, Provenance: workers.Provenance{Provider: "codex"}},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-no-session-ref" }, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "codex"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || !target.read {
		t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
	}
	progress := response.Error.Details["progress"].(map[string]any)
	activity := progress["lastObservedProviderActivity"].(map[string]any)
	if _, exists := activity["providerSessionObserved"]; exists {
		t.Fatalf("unexpected provider session observation = %#v", activity)
	}
	if activity["kind"] != "MESSAGE" || activity["phase"] != "DELTA" || len(activity) != 3 {
		t.Fatalf("retained provider activity = %#v", activity)
	}
	observedAt, ok := activity["observedAt"].(time.Time)
	if !ok || !observedAt.Equal(now) {
		t.Fatalf("observedAt = %#v", activity["observedAt"])
	}
}

func TestSubagentPiModelConnectionMCPContent(t *testing.T) {
	const secret = "private-model-endpoint-token"
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		Message:       "Connection error. " + secret,
		FailureReason: string(workers.WorkFailureTypeMisconfigured),
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-pi-model-connection" }, mcpfactorysession.SubagentInput{Prompt: "Read README", Provider: "pi"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_misconfigured" || response.Error.Retryable {
		t.Fatalf("response = %#v", response)
	}
	if action, ok := response.Error.Details["suggestedAction"].(string); !ok ||
		!strings.HasPrefix(action, "Check that Pi's selected model endpoint is running and reachable.") ||
		!strings.Contains(action, "Pi setup and capabilities") || !strings.Contains(action, "pi --version") {
		t.Fatalf("suggested action = %#v", response.Error.Details["suggestedAction"])
	}
	assertPiModelConnectionMCPContent(t, response, target, secret)
}

func assertPiModelConnectionMCPContent(t *testing.T, response mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], target *subagentTargetFake, secret string) {
	t.Helper()
	toolResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := mcpfactorysession.MarshalDomainErrorCallToolResultJSON(toolResponse)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) != 1 ||
		!strings.Contains(result.Content[0].Text, "Pi's selected model endpoint is running and reachable") {
		t.Fatalf("MCP content = %s", encoded)
	}
	var structured mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult]
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Error == nil || structured.Error.Code != response.Error.Code || structured.Error.Details["failureReason"] != string(workers.WorkFailureTypeMisconfigured) || !target.closed {
		t.Fatalf("MCP structured content = %s", encoded)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("MCP response leaked provider text: %s", encoded)
	}
}
