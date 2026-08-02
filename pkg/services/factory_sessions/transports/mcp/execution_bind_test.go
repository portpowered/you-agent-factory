package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

const internalLeakProbePath = "pkg/services/factory_sessions/internal/sessionstore"

func TestBind_FakeExecutionRootInvokedThroughCanonicalListSessionsTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopePersisted {
				t.Fatalf("scope = %q, want persisted", request.Scope)
			}
			return factorysessions.ListSessionsResult{Scope: factorysessions.SessionListScopePersisted}, nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{"scope":"persisted"}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"scope":"persisted"`) {
		t.Fatalf("CallTool(list_sessions) = %s, want persisted scope result", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalGetSessionTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID != runningSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, runningSessionID)
			}
			return runningSessionRead(), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+runningSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"status":"RUNNING"`) || !strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) {
		t.Fatalf("CallTool(get_session) = %s, want running session read model", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalListDispatchesTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != successSessionID {
				t.Fatalf("sessionId = %q, want %q", request.SessionID, successSessionID)
			}
			if request.Filters.Phase != "execution" || request.Filters.Status != factorysessions.DispatchStatus("COMPLETED") {
				t.Fatalf("filters = %#v, want execution/COMPLETED", request.Filters)
			}
			return factorysessions.ListDispatchesResult{
				SessionID: successSessionID,
				Dispatches: []factorysessions.DispatchSummary{{
					ID:     "dispatch-001",
					Phase:  "execution",
					Status: factorysessions.DispatchStatus("COMPLETED"),
				}},
			}, nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolListDispatches,
		json.RawMessage(`{"sessionId":"`+successSessionID+`","phase":"execution","status":"COMPLETED"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_dispatches) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"id":"dispatch-001"`) || !strings.Contains(string(raw), `"sessionId":"`+successSessionID+`"`) {
		t.Fatalf("CallTool(list_dispatches) = %s, want encoded dispatch list", raw)
	}
}

func TestBind_ReadListToolsInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolGetSession, json.RawMessage(`{"sessionId":`))
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for invalid JSON decode")
	}
}

func TestBind_ReadListToolsValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	preparation := mcpRequestPreparation{
		listSessions: func(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
			return factorysessions.ListSessionsRequest{}, errors.New(`unsupported Factory Session scope "workspace"`)
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   preparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{"scope":"workspace"}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for validation failure")
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalStartAsyncTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		startAsync: func(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			if request.RequestID != "req-js-run-n-001" {
				t.Fatalf("requestId = %q, want req-js-run-n-001", request.RequestID)
			}
			if request.Source.FactoryID != "customer-support-triage" {
				t.Fatalf("factoryId = %q, want customer-support-triage", request.Source.FactoryID)
			}
			return runningAsyncStart(), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolStartAsync,
		json.RawMessage(`{"requestId":"req-js-run-n-001","source":{"kind":"FACTORY_ID","factoryId":"customer-support-triage"}}`),
	)
	if err != nil {
		t.Fatalf("CallTool(start_async) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) ||
		!strings.Contains(string(raw), `"status":"RUNNING"`) {
		t.Fatalf("CallTool(start_async) = %s, want encoded async start response", raw)
	}
}

func TestBind_FakeExecutionRootInvokedThroughCanonicalControlTool(t *testing.T) {
	t.Parallel()

	const pauseReason = "host maintenance"
	var invoked bool
	fake := fakeExecutionRoot{
		invoked: &invoked,
		pause: func(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if sessionID != runningSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, runningSessionID)
			}
			if request.Reason != pauseReason {
				t.Fatalf("reason = %q, want %q", request.Reason, pauseReason)
			}
			return acceptedControl(runningSessionID, "PAUSE", factorysessions.LifecycleStatusPaused), nil
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolControl,
		json.RawMessage(`{"sessionId":"`+runningSessionID+`","operation":"PAUSE","reason":"`+pauseReason+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(control) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake execution root was not invoked")
	}
	if !strings.Contains(string(raw), `"outcome":"ACCEPTED"`) ||
		!strings.Contains(string(raw), `"status":"PAUSED"`) ||
		!strings.Contains(string(raw), `"sessionId":"`+runningSessionID+`"`) {
		t.Fatalf("CallTool(control) = %s, want encoded lifecycle control response", raw)
	}
}

func TestBind_StartControlToolsInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(context.Background(), mcpfactorysession.ToolStartAsync, json.RawMessage(`{"requestId":`))
	if err != nil {
		t.Fatalf("CallTool(start_async) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for invalid JSON decode")
	}
}

func TestBind_StartControlToolsValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	preparation := mcpRequestPreparation{
		start: func(factorysessions.StartRequest) (factorysessions.StartRequest, error) {
			return factorysessions.StartRequest{}, errors.New("factory session source factoryId is required")
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   preparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolStartAsync,
		json.RawMessage(`{"requestId":"req-malformed-001","source":{"kind":"FACTORY_ID"}}`),
	)
	if err != nil {
		t.Fatalf("CallTool(start_async) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake execution root was invoked for validation failure")
	}
}

func TestBind_GetSessionTypedNotFoundErrorReturnsToolErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		getSession: func(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			if sessionID != missingSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, missingSessionID)
			}
			return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+missingSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.session.not_found",
		false,
		missingSessionID,
	)
	if envelope.Message != "factory session not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "factory session not found", envelope)
	}
}

func TestBind_ListDispatchesExecutionValidationErrorReturnsBadRequestEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != successSessionID {
				t.Fatalf("sessionId = %q, want %q", request.SessionID, successSessionID)
			}
			return factorysessions.ListDispatchesResult{}, &factorysessions.ExecutionValidationError{
				Field:   "status",
				Message: "invalid status",
			}
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolListDispatches,
		json.RawMessage(`{"sessionId":"`+successSessionID+`","status":"BROKEN"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_dispatches) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "invalid status") {
		t.Fatalf("error.message = %q, want validation detail; envelope = %#v", envelope.Message, envelope)
	}
	if envelope.Details == nil || envelope.Details["field"] != "status" {
		t.Fatalf("error.details = %#v, want field=status", envelope.Details)
	}
}

func TestBind_UnmappedRootErrorDoesNotLeakInternalPackagePaths(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		getSession: func(_ context.Context, _ string) (factorysessions.SessionReadResult, error) {
			return factorysessions.SessionReadResult{}, fmt.Errorf(
				"%s: connection reset\ngoroutine 1 [running]:\nmain.main()",
				internalLeakProbePath,
			)
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	raw, err := operation(
		context.Background(),
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+missingSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	assertEnvelopeDoesNotLeakInternalPaths(t, raw)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.execution.internal",
		false,
		"",
	)
	if envelope.Message != "factory session execution failed" {
		t.Fatalf("error.message = %q, want sanitized internal message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_GetSessionContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fakeExecutionRoot{invoked: &invoked},
		Prepare:   canonicalMCPRequestPreparation,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		mcpfactorysession.ToolGetSession,
		json.RawMessage(`{"sessionId":"`+runningSessionID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake execution root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.request.canceled",
		false,
		"",
	)
	if envelope.Message != "factory session request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_GetSessionContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeExecutionRoot{
		getSession: func(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return factorysessions.SessionReadResult{}, ctx.Err()
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			mcpfactorysession.ToolGetSession,
			json.RawMessage(`{"sessionId":"`+runningSessionID+`"}`),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake execution root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(get_session) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(get_session) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.request.canceled",
		false,
		"",
	)
	if envelope.Message != "factory session request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_StartAsyncContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeExecutionRoot{
		startAsync: func(ctx context.Context, _ factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			<-ctx.Done()
			return factorysessions.AsyncStartResult{}, ctx.Err()
		},
	}
	operation := mcpfactorysession.Bind(mcpfactorysession.RootDependencies{
		Execution: fake,
		Prepare:   canonicalMCPRequestPreparation,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		mcpfactorysession.ToolStartAsync,
		mustStartAsyncJSON(t),
	)
	if err != nil {
		t.Fatalf("CallTool(start_async) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_session.request.timed_out",
		true,
		"",
	)
	if envelope.Message != "factory session request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}

func mustStartAsyncJSON(t *testing.T) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(asyncRunningExecutionRequest())
	if err != nil {
		t.Fatalf("marshal start async input: %v", err)
	}
	return payload
}

type fakeExecutionRoot struct {
	mcpfactorysession.DurableExecution
	invoked         *bool
	listSessions    func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	getSession      func(context.Context, string) (factorysessions.SessionReadResult, error)
	queryDispatches func(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	startAsync      func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	pause           func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
}

func (root fakeExecutionRoot) markInvoked() {
	if root.invoked != nil {
		*root.invoked = true
	}
}

func (root fakeExecutionRoot) ListSessions(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	root.markInvoked()
	if root.listSessions == nil {
		panic("unexpected ListSessions on fake execution root")
	}
	return root.listSessions(ctx, request)
}

func (root fakeExecutionRoot) GetSession(
	ctx context.Context,
	sessionID string,
) (factorysessions.SessionReadResult, error) {
	root.markInvoked()
	if root.getSession == nil {
		panic("unexpected GetSession on fake execution root")
	}
	return root.getSession(ctx, sessionID)
}

func (root fakeExecutionRoot) QueryDispatches(
	ctx context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	root.markInvoked()
	if root.queryDispatches == nil {
		panic("unexpected QueryDispatches on fake execution root")
	}
	return root.queryDispatches(ctx, request)
}

func (root fakeExecutionRoot) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	root.markInvoked()
	if root.startAsync == nil {
		panic("unexpected StartAsync on fake execution root")
	}
	return root.startAsync(ctx, request)
}

func (root fakeExecutionRoot) Pause(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	root.markInvoked()
	if root.pause == nil {
		panic("unexpected Pause on fake execution root")
	}
	return root.pause(ctx, sessionID, request)
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantSessionID string,
) *mcpfactorysession.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                     `json:"result"`
		Error  *mcpfactorysession.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if wantSessionID != "" && response.Error.SessionID != wantSessionID {
		t.Fatalf("error.sessionId = %q, want %q; envelope = %#v", response.Error.SessionID, wantSessionID, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

func assertEnvelopeDoesNotLeakInternalPaths(t *testing.T, raw json.RawMessage) {
	t.Helper()
	if strings.Contains(string(raw), internalLeakProbePath) {
		t.Fatalf("tool response leaks internal package path: %s", raw)
	}
}
