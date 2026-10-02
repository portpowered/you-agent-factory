package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
		actionText []string
	}{
		{
			name: "throttled", reason: string(workers.WorkFailureTypeThrottled),
			code: "factory_session.subagent.provider_throttled", retryable: true,
			actionText: []string{
				"Wait for the provider usage or capacity limit to clear",
				"select another available configured model or provider",
			},
		},
		{
			name: "permanent bad request", reason: string(workers.WorkFailureTypePermanentBadRequest),
			code:       "factory_session.subagent.provider_request_rejected",
			actionText: []string{"selected model against the provider's advertised models and request settings"},
		},
		{
			name: "internal server error", reason: string(workers.WorkFailureTypeInternalServerError),
			code: "factory_session.subagent.provider_internal_error", retryable: true,
			actionText: []string{"Check provider status and logs"},
		},
		{
			name: "unknown", reason: string(workers.WorkFailureTypeUnknown),
			code:       "factory_session.subagent.provider_unknown_failure",
			actionText: []string{"Check provider logs and configuration"},
		},
		{
			name:       "empty reason",
			code:       "factory_session.subagent.execution_failed",
			actionText: []string{"Check provider logs and configuration"},
		},
		{
			// An unrecognized reason has no classification-specific remedy, so the
			// shared helper proves only the enrichment guidance.
			name: "unrecognized reason", reason: "unrecognized-" + secret,
			code: "factory_session.subagent.execution_failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSubagentTerminalFailureClassification(t, tt.reason, tt.code, tt.retryable, tt.actionText, secret)
		})
	}
}

func assertSubagentTerminalFailureClassification(t *testing.T, reason, code string, retryable bool, actionText []string, secret string) {
	t.Helper()
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive output " + secret,
		FailureReason: reason,
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-classification" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("response = %#v", response)
	}
	if got := response.Error; got.Code != code || got.Retryable != retryable {
		t.Fatalf("code = %q, retryable = %t; want %q, %t", got.Code, got.Retryable, code, retryable)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok {
		t.Fatalf("suggestedAction = %#v", response.Error.Details["suggestedAction"])
	}
	for _, want := range actionText {
		if !strings.Contains(action, want) {
			t.Fatalf("suggestedAction = %q, want recovery text %q", action, want)
		}
	}
	// Cleanup enrichment must keep partial-edit inspection and provider log
	// correlation ahead of every classified remedy.
	if !strings.Contains(action, "Inspect the workspace for partial edits") ||
		!strings.Contains(action, "check provider logs") {
		t.Fatalf("suggestedAction = %q, want partial-edit inspection and provider log guidance", action)
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-request" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-failure" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-empty" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "activity-no-session-ref" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "codex"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-pi-model-connection" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Read README", Provider: "pi"})
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

func TestSubagentTimeoutWithObservedProviderErrorRefinesMessageAndAction(t *testing.T) {
	const secret = "private provider error payload"
	now := time.Now()
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
			Status:    factorysessions.InvocationTerminalStatusTimedOut,
			RequestID: "request-provider-error",
			TraceID:   "trace-provider-error",
			WorkID:    "work-provider-error",
		}},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindError, Phase: workers.PhaseFailed, RecordedAt: now.Add(-time.Second), Provenance: workers.Provenance{Provider: "opencode"}, Payload: json.RawMessage(`{"error":"` + secret + `"}`)},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-provider-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || !target.read {
		t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
	}
	progress := response.Error.Details["progress"].(map[string]any)
	activity := progress["lastObservedProviderActivity"].(map[string]any)
	if activity["kind"] != "ERROR" || activity["phase"] != "FAILED" || activity["providerSessionObserved"] == true {
		t.Fatalf("last provider activity = %#v", activity)
	}
	if !strings.Contains(response.Error.Message, "provider error") {
		t.Fatalf("timeout message = %q, want provider error evidence", response.Error.Message)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok {
		t.Fatalf("suggestedAction = %#v", response.Error.Details["suggestedAction"])
	}
	if strings.Contains(action, "use another configured model or a longer timeout") {
		t.Fatalf("provider-error action keeps the default retry remedy: %#v", action)
	}
	if !strings.Contains(action, "no provider session reference was observed") || !strings.Contains(action, "longer timeout may not resolve") {
		t.Fatalf("provider-error action = %#v", action)
	}
	encoded, err := json.Marshal(response.Error)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("provider-error timeout leaked provider data: %s, %v", encoded, err)
	}
}

func TestSubagentInvokeDeadlineWithObservedProviderErrorRefinesMessageAndAction(t *testing.T) {
	const secret = "private provider error payload"
	now := time.Now()
	timeout := int64(60_000)
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeErr: fmt.Errorf("private: %w", context.DeadlineExceeded)},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindError, Phase: workers.PhaseFailed, RecordedAt: now.Add(-time.Second), Provenance: workers.Provenance{Provider: "opencode"}, Payload: json.RawMessage(`{"error":"` + secret + `"}`)},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-deadline-provider-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode", TimeoutMillis: &timeout})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || !target.read {
		t.Fatalf("deadline timeout and cleanup = %#v, target = %#v", response, target)
	}
	if !strings.Contains(response.Error.Message, "provider error") {
		t.Fatalf("deadline timeout message = %q, want provider error evidence", response.Error.Message)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok {
		t.Fatalf("suggestedAction = %#v", response.Error.Details["suggestedAction"])
	}
	if strings.Contains(action, "use another configured model or a longer timeout") || !strings.Contains(action, "no provider session reference was observed") {
		t.Fatalf("deadline provider-error action = %#v", action)
	}
	encoded, err := json.Marshal(response.Error)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("deadline provider-error timeout leaked provider data: %s, %v", encoded, err)
	}
}

// A real 20-minute subagent timeout retained a terminal provider error together
// with a provider session reference. The observation is still timing evidence,
// so it must refine the diagnostic instead of leaving the misleading slow-model
// remedy in place, and it must not claim the reference was absent.
func TestSubagentTimeoutWithObservedProviderSessionErrorRefinesMessageAndAction(t *testing.T) {
	const secret = "private provider session reference and error payload"
	now := time.Now()
	timeout := int64(60_000)
	for _, test := range []struct {
		name       string
		invokeErr  error
		invoke     *factorysessions.InvocationResult
		wantDetail map[string]string
	}{
		{
			name: "terminal timed out",
			invoke: &factorysessions.InvocationResult{
				Status:    factorysessions.InvocationTerminalStatusTimedOut,
				ErrorCode: "INVOCATION_TIMED_OUT",
				Message:   "private provider output " + secret,
				RequestID: "request-session-error",
				TraceID:   "trace-session-error",
				WorkID:    "work-session-error",
			},
			wantDetail: map[string]string{
				"requestId": "request-session-error", "traceId": "trace-session-error", "workId": "work-session-error",
			},
		},
		{name: "deadline exceeded", invokeErr: fmt.Errorf("private: %w", context.DeadlineExceeded)},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := &subagentActivityTarget{
				subagentTargetFake: &subagentTargetFake{invokeErr: test.invokeErr, invokeResult: test.invoke},
				events: []factorysessions.FactoryResponseEvent{{
					Kind: workers.KindError, Phase: workers.PhaseFailed, RecordedAt: now.Add(-time.Second),
					Provenance:         workers.Provenance{Provider: secret},
					ProviderSessionRef: secret,
					Payload:            json.RawMessage(`{"error":"` + secret + `"}`),
				}},
			}
			response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-session-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode", TimeoutMillis: &timeout})
			if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || !target.read {
				t.Fatalf("timeout and cleanup = %#v, target = %#v", response, target)
			}
			assertObservedProviderSessionErrorDiagnostic(t, response, secret)
			for key, want := range test.wantDetail {
				if response.Error.Details[key] != want {
					t.Fatalf("%s = %#v, want %q", key, response.Error.Details[key], want)
				}
			}
		})
	}
}

func assertObservedProviderSessionErrorDiagnostic(t *testing.T, response mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], secret string) {
	t.Helper()
	envelope := response.Error
	if envelope.Retryable || envelope.Details["partialEffectsPossible"] != true || envelope.SessionID != "session-1" {
		t.Fatalf("timeout diagnostic = %#v", envelope)
	}
	assertObservedProviderSessionActivity(t, envelope)
	assertObservedProviderSessionAction(t, envelope)
	encoded, err := json.Marshal(response)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("observed-session timeout leaked provider data: %s, %v", encoded, err)
	}
	assertSubagentObservedSessionErrorMCPContent(t, response, encoded, secret)
}

// A bounded snapshot of the observed activity carries fixed vocabulary only, so
// the provider session reference itself never reaches the envelope.
func assertObservedProviderSessionActivity(t *testing.T, envelope *mcpfactorysession.ToolErrorEnvelope) {
	t.Helper()
	if !strings.Contains(envelope.Message, "provider error was observed") {
		t.Fatalf("timeout message = %q, want provider error evidence", envelope.Message)
	}
	activity, ok := envelope.Details["progress"].(map[string]any)["lastObservedProviderActivity"].(map[string]any)
	if !ok || activity["kind"] != "ERROR" || activity["phase"] != "FAILED" || activity["providerSessionObserved"] != true || len(activity) != 4 {
		t.Fatalf("last provider activity = %#v", activity)
	}
}

func assertObservedProviderSessionAction(t *testing.T, envelope *mcpfactorysession.ToolErrorEnvelope) {
	t.Helper()
	action, ok := envelope.Details["suggestedAction"].(string)
	if !ok || !strings.Contains(action, "a provider session reference was observed") || !strings.Contains(action, "longer timeout may not resolve") {
		t.Fatalf("observed-session action = %#v", envelope.Details["suggestedAction"])
	}
	if strings.Contains(action, "no provider session reference was observed") || strings.Contains(action, "use another configured model or a longer timeout") {
		t.Fatalf("observed-session action asserts absence or keeps the default remedy: %#v", action)
	}
}

// The human-readable MCP text and structuredContent must carry the same refined
// timeout evidence as the typed response.
func assertSubagentObservedSessionErrorMCPContent(t *testing.T, response mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], toolResponse []byte, secret string) {
	t.Helper()
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
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text != response.Error.Message {
		t.Fatalf("MCP content = %s", encoded)
	}
	var structured mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult]
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Error == nil || structured.Error.Code != response.Error.Code ||
		structured.Error.Details["suggestedAction"] != response.Error.Details["suggestedAction"] {
		t.Fatalf("MCP structured content = %s", encoded)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("MCP response leaked provider text: %s", encoded)
	}
}

// An earlier provider error in the retained activity must not turn a completed
// invocation into a failure.
func TestSubagentCompletedInvocationIgnoresEarlierObservedProviderError(t *testing.T) {
	const secret = "private provider session reference"
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{},
		events: []factorysessions.FactoryResponseEvent{{
			Kind: workers.KindError, Phase: workers.PhaseFailed, RecordedAt: time.Now(),
			Provenance:         workers.Provenance{Provider: "opencode"},
			ProviderSessionRef: secret,
		}},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-completed-after-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error != nil || response.Result == nil || response.Result.Text != "subagent answer" {
		t.Fatalf("completed response = %#v", response)
	}
	if target.read || target.getCalls != 0 {
		t.Fatalf("completed invocation read timeout evidence: read=%t getCalls=%d", target.read, target.getCalls)
	}
	if !target.closed {
		t.Fatal("completed invocation skipped Factory Session cleanup")
	}
	if encoded, err := json.Marshal(response); err != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("completed response leaked provider data: %s, %v", encoded, err)
	}
}

// A later non-error activity is the last observation, so the default retry
// guidance stays.
func TestSubagentTimeoutWithLaterNonErrorActivityKeepsDefaultAction(t *testing.T) {
	now := time.Now()
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindError, Phase: workers.PhaseFailed, RecordedAt: now.Add(-time.Second), Provenance: workers.Provenance{Provider: "opencode"}},
			{Kind: workers.KindReasoning, Phase: workers.PhaseDelta, RecordedAt: now, Provenance: workers.Provenance{Provider: "opencode"}},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-later-reasoning" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" {
		t.Fatalf("timeout response = %#v", response)
	}
	if strings.Contains(response.Error.Message, "provider error was observed") {
		t.Fatalf("superseded error refined the message: %q", response.Error.Message)
	}
	activity, ok := response.Error.Details["progress"].(map[string]any)["lastObservedProviderActivity"].(map[string]any)
	if !ok || activity["kind"] != "REASONING" || activity["phase"] != "DELTA" {
		t.Fatalf("last provider activity = %#v", activity)
	}
	if action, ok := response.Error.Details["suggestedAction"].(string); !ok || !strings.Contains(action, "use another configured model or a longer timeout") {
		t.Fatalf("later non-error action = %#v", response.Error.Details["suggestedAction"])
	}
}

func TestSubagentTimeoutWithNonTerminalProviderErrorKeepsDefaultAction(t *testing.T) {
	now := time.Now()
	target := &subagentActivityTarget{
		subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		events: []factorysessions.FactoryResponseEvent{
			{Kind: workers.KindError, Phase: workers.PhaseUpdated, RecordedAt: now.Add(-time.Second), Provenance: workers.Provenance{Provider: "opencode"}},
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-nonterminal-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" {
		t.Fatalf("timeout response = %#v", response)
	}
	if strings.Contains(response.Error.Message, "provider error was observed") {
		t.Fatalf("non-terminal error refined the message: %q", response.Error.Message)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok || !strings.Contains(action, "use another configured model or a longer timeout") {
		t.Fatalf("non-terminal error action = %#v", response.Error.Details["suggestedAction"])
	}
}

func TestSubagentQualifiedOpenCodeModelSelectsProvider(t *testing.T) {
	const model = "opencode/muse-spark-1.3-contributor-free"
	for _, test := range []struct {
		name     string
		provider string
	}{
		{name: "provider inferred"},
		{name: "provider explicit", provider: "opencode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := &subagentTargetFake{}
			response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-model" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
				Prompt: "Return OK", Provider: test.provider, Model: model,
			})
			if response.Error != nil || response.Result == nil {
				t.Fatalf("Subagent response = %#v", response)
			}
			if got := target.invoke.Args["workerProvider"]; got != "opencode" {
				t.Fatalf("workerProvider = %#v, want opencode", got)
			}
			if got := target.invoke.Args["workerModel"]; got != model {
				t.Fatalf("workerModel = %#v, want %q", got, model)
			}
			if !target.closed {
				t.Fatal("Factory Session was not closed")
			}
		})
	}
}

func TestSubagentQualifiedOpenCodeModelRejectsConflictingProviderBeforeStart(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-model-conflict" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Return OK", Provider: "codex", Model: "opencode/muse-spark-1.3-contributor-free",
	})
	if response.Result != nil || response.Error == nil || response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("Subagent response = %#v, want bad request", response)
	}
	if !strings.Contains(response.Error.Message, "requires provider opencode") {
		t.Fatalf("error message = %q, want provider conflict", response.Error.Message)
	}
	if target.started {
		t.Fatal("conflicting provider started a Factory Session")
	}
}

// subagentProviderResolverFake stands in for the Providers-owned identity
// resolver bound by production composition. It records every selection the
// tool asks the catalog to canonicalize so tests can prove which selections
// reach the catalog and which keep operator defaults.
type subagentProviderResolverFake struct {
	canonical map[string]string
	err       error
	asked     []string
}

func (fake *subagentProviderResolverFake) resolve(_ context.Context, identity string) (string, error) {
	fake.asked = append(fake.asked, identity)
	if fake.err != nil {
		return "", fake.err
	}
	canonical, ok := fake.canonical[identity]
	if !ok {
		return "", fmt.Errorf("%w: %q", providers.ErrUnknownProvider, identity)
	}
	return canonical, nil
}

// TestSubagentRejectsUnknownProviderSelectionBeforeStart proves an explicitly
// invalid provider identifier is an actionable validation failure instead of a
// dispatched run that ends as provider_unknown_failure/INVOCATION_RUNTIME_FAILURE.
func TestSubagentRejectsUnknownProviderSelectionBeforeStart(t *testing.T) {
	const provider = "deliberately-invalid-probe-provider"
	target := &subagentTargetFake{}
	resolver := &subagentProviderResolverFake{}
	response := mcpfactorysession.Subagent(
		context.Background(), target, "C:/project",
		func() string { return "request-unknown-provider" }, resolver.resolve,
		mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: provider},
	)
	if response.Result != nil || response.Error == nil {
		t.Fatalf("Subagent response = %#v, want typed provider error", response)
	}
	if got := response.Error; got.Code != "factory_session.subagent.provider_not_found" || got.Retryable {
		t.Fatalf("code = %q, retryable = %t; want provider_not_found, false", got.Code, got.Retryable)
	}
	if !strings.Contains(response.Error.Message, "not a known model provider") ||
		!strings.Contains(response.Error.Message, "no Factory Session was started") {
		t.Fatalf("error message = %q, want actionable not-found text", response.Error.Message)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok || !strings.Contains(action, "omit provider to use operator defaults") {
		t.Fatalf("suggestedAction = %#v", response.Error.Details["suggestedAction"])
	}
	assertSubagentProviderRejectedBeforeStart(t, target)
	if len(resolver.asked) != 1 || resolver.asked[0] != provider {
		t.Fatalf("catalog lookups = %#v, want one lookup of %q", resolver.asked, provider)
	}
	assertSubagentProviderNotFoundMCPContent(t, response)
}

// assertSubagentProviderRejectedBeforeStart proves the rejection happens before
// any Factory Session exists, so the envelope must not claim a session identity,
// partial execution, a terminal status, or an invocation code.
func assertSubagentProviderRejectedBeforeStart(t *testing.T, target *subagentTargetFake) {
	t.Helper()
	if target.started || target.closed || target.getCalls != 0 || target.control.Operation != "" {
		t.Fatalf("unknown provider reached the runtime: %#v", target)
	}
	if len(target.invoke.Args) != 0 || target.invoke.SessionID != "" {
		t.Fatalf("unknown provider dispatched work: %#v", target.invoke)
	}
}

// assertSubagentProviderNotFoundMCPContent proves the public MCP encoding keeps
// human text and structured content in parity for the new typed error without
// exposing a private provider reason.
func assertSubagentProviderNotFoundMCPContent(t *testing.T, response mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult]) {
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
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text != response.Error.Message {
		t.Fatalf("MCP content = %s", encoded)
	}
	var structured mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult]
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Error == nil || structured.Error.Code != response.Error.Code ||
		structured.Error.Retryable || structured.Error.SessionID != "" ||
		structured.Error.Details["reason"] != "UNKNOWN_PROVIDER" {
		t.Fatalf("MCP structured content = %s", encoded)
	}
	for _, absent := range []string{"partialEffectsPossible", "sessionClosed", "sessionIdPurpose", "status", "invocationCode", "failureReason"} {
		if _, present := structured.Error.Details[absent]; present {
			t.Fatalf("MCP structured content claims %q for a request that never started: %s", absent, encoded)
		}
	}
}

// TestSubagentProviderSelectionPreservesAliasesAndOperatorDefaults proves the
// authoritative catalog decides accepted selections: supported aliases are
// canonicalized before dispatch, dynamically registered providers resolve, and
// an omitted provider never consults the catalog so operator defaults apply.
func TestSubagentProviderSelectionPreservesAliasesAndOperatorDefaults(t *testing.T) {
	for _, test := range []struct {
		name         string
		provider     string
		model        string
		wantResolved string
		wantArgument string
	}{
		{name: "canonical id", provider: "codex", wantResolved: "codex", wantArgument: "codex"},
		{name: "compatibility alias", provider: "openai", wantResolved: "codex", wantArgument: "codex"},
		{name: "case insensitive alias", provider: "OpenCode", wantResolved: "opencode", wantArgument: "opencode"},
		{name: "qualified model with operator-configured alias", provider: "operator-open-code", model: "opencode/space-bunny-free", wantResolved: "opencode", wantArgument: "opencode"},
		{name: "dynamically registered provider", provider: "operator-acp", wantResolved: "operator-acp", wantArgument: "operator-acp"},
		{name: "omitted provider keeps defaults", provider: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := &subagentTargetFake{}
			resolver := &subagentProviderResolverFake{canonical: map[string]string{
				"codex": "codex", "openai": "codex", "OpenCode": "opencode", "operator-open-code": "opencode", "operator-acp": "operator-acp",
			}}
			response := mcpfactorysession.Subagent(
				context.Background(), target, "C:/project",
				func() string { return "request-provider-selection" }, resolver.resolve,
				mcpfactorysession.SubagentInput{Prompt: "Return OK", Provider: test.provider, Model: test.model},
			)
			if response.Error != nil || response.Result == nil {
				t.Fatalf("Subagent response = %#v", response)
			}
			argument, forwarded := target.invoke.Args["workerProvider"]
			if test.wantArgument == "" {
				if forwarded || len(resolver.asked) != 0 {
					t.Fatalf("workerProvider = %#v (present %t), catalog lookups = %#v; want operator defaults", argument, forwarded, resolver.asked)
				}
				return
			}
			if argument != test.wantArgument {
				t.Fatalf("workerProvider = %#v, want %q", argument, test.wantArgument)
			}
			if len(resolver.asked) != 1 || resolver.asked[0] != test.provider {
				t.Fatalf("catalog lookups = %#v, want one lookup of %q", resolver.asked, test.provider)
			}
			if !target.closed {
				t.Fatal("Factory Session was not closed")
			}
		})
	}
}
