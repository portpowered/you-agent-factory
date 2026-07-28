package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryrunmcp "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/mcp"
)

const internalLeakProbePath = "pkg/services/factory_runtime/internal/services/instance_host"

func TestBind_ControlPauseSuccessResultParity(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		controlPause: func(_ context.Context, request factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			if request != (factoryruntime.PauseRequest{}) {
				t.Fatalf("pause request = %#v, want empty PauseRequest", request)
			}
			return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeNoOp}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolControlPause, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}

	var response factoryrunmcp.ToolResponse[factoryruntime.PauseResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatalf("tool response = %s, want result envelope", raw)
	}
	if response.Result.Outcome != factoryruntime.ControlOutcomeNoOp {
		t.Fatalf("result.Outcome = %q, want NO_OP", response.Result.Outcome)
	}
}

func TestBind_ObserveSuccessResultParity(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		observe: func(_ context.Context, request factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			if request.Scope != factoryruntime.ObservationScopeProgress {
				t.Fatalf("scope = %q, want PROGRESS", request.Scope)
			}
			return factoryruntime.ObserveResult{
				Observation: factoryruntime.Observation{
					Status: factoryruntime.ObservationStatusActive,
					Progress: factoryruntime.ObservationProgress{
						InFlightDispatchCount: 2,
						TickCount:             7,
						TotalWorkCount:        11,
					},
				},
			}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolObserve,
		json.RawMessage(`{"scope":"PROGRESS"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}

	var response factoryrunmcp.ToolResponse[factoryruntime.ObserveResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatalf("tool response = %s, want result envelope", raw)
	}
	if response.Result.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("result.Observation.Status = %q, want ACTIVE", response.Result.Observation.Status)
	}
	if response.Result.Observation.Progress.TickCount != 7 {
		t.Fatalf("result.Observation.Progress.TickCount = %d, want 7", response.Result.Observation.Progress.TickCount)
	}
}

func TestBind_ObserveEmptyInputDecodesToDefaultRootRequest(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		observe: func(_ context.Context, request factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			if request.Scope != "" {
				t.Fatalf("scope = %q, want empty scope for omitted input", request.Scope)
			}
			return factoryruntime.ObserveResult{
				Observation: factoryruntime.Observation{Status: factoryruntime.ObservationStatusIdle},
			}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolObserve, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}
	if !strings.Contains(string(raw), `"Status":"IDLE"`) {
		t.Fatalf("CallTool(observe) = %s, want encoded idle observation", raw)
	}
}

func TestBind_ControlPauseInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(context.Background(), factoryrunmcp.ToolControlPause, json.RawMessage(`{"unexpected":`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for invalid JSON decode")
	}
}

func TestBind_ObserveInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(context.Background(), factoryrunmcp.ToolObserve, json.RawMessage(`{"scope":`))
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for invalid JSON decode")
	}
}

func TestBind_ObserveValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(context.Background(), factoryrunmcp.ToolObserve, json.RawMessage(`{"scope":"WORKSPACE"}`))
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for validation failure")
	}
}

func TestBind_PlanDispatchSuccessResultParity(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		planDispatch: func(_ context.Context, request factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
			want := factoryruntime.PlanDispatchRequest{
				DispatchID:      "dispatch-1",
				CorrelationID:   "corr-1",
				WorkIDs:         []string{"work-1"},
				WorkstationName: "ws-alpha",
				WorkerType:      "coder",
				ReplayKey:       "replay-1",
			}
			if !reflect.DeepEqual(request, want) {
				t.Fatalf("plan dispatch request = %#v, want %#v", request, want)
			}
			return factoryruntime.PlanDispatchResult{
				Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
				DispatchID:    "dispatch-1",
				CorrelationID: "corr-1",
			}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolPlanDispatch,
		json.RawMessage(`{
			"dispatchId":"dispatch-1",
			"correlationId":"corr-1",
			"workIds":["work-1"],
			"workstationName":"ws-alpha",
			"workerType":"coder",
			"replayKey":"replay-1"
		}`),
	)
	if err != nil {
		t.Fatalf("CallTool(plan_dispatch) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}

	var response factoryrunmcp.ToolResponse[factoryruntime.PlanDispatchResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatalf("tool response = %s, want result envelope", raw)
	}
	if response.Result.Outcome != factoryruntime.DispatchPlanOutcomeAccepted {
		t.Fatalf("result.Outcome = %q, want ACCEPTED", response.Result.Outcome)
	}
	if response.Result.DispatchID != "dispatch-1" || response.Result.CorrelationID != "corr-1" {
		t.Fatalf("result = %#v, want dispatch-1/corr-1 identities", response.Result)
	}
}

func TestBind_AcceptDispatchResultSuccessResultParity(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		acceptDispatchResult: func(_ context.Context, request factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
			want := factoryruntime.AcceptDispatchResultRequest{
				DispatchID:    "dispatch-1",
				CorrelationID: "corr-1",
				WorkID:        "work-1",
				ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
			}
			if request != want {
				t.Fatalf("accept dispatch result request = %#v, want %#v", request, want)
			}
			return factoryruntime.AcceptDispatchResultResult{
				Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
				DispatchID:    "dispatch-1",
				CorrelationID: "corr-1",
			}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolAcceptDispatchResult,
		json.RawMessage(`{
			"dispatchId":"dispatch-1",
			"correlationId":"corr-1",
			"workId":"work-1",
			"resultOutcome":"SUCCESS"
		}`),
	)
	if err != nil {
		t.Fatalf("CallTool(accept_dispatch_result) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}

	var response factoryrunmcp.ToolResponse[factoryruntime.AcceptDispatchResultResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatalf("tool response = %s, want result envelope", raw)
	}
	if response.Result.Outcome != factoryruntime.DispatchPlanOutcomeRetired {
		t.Fatalf("result.Outcome = %q, want RETIRED", response.Result.Outcome)
	}
	if response.Result.DispatchID != "dispatch-1" || response.Result.CorrelationID != "corr-1" {
		t.Fatalf("result = %#v, want dispatch-1/corr-1 identities", response.Result)
	}
}

func TestBind_PlanDispatchInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(context.Background(), factoryrunmcp.ToolPlanDispatch, json.RawMessage(`{"dispatchId":`))
	if err != nil {
		t.Fatalf("CallTool(plan_dispatch) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for invalid JSON decode")
	}
}

func TestBind_AcceptDispatchResultInvalidJSONDecodeReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(context.Background(), factoryrunmcp.ToolAcceptDispatchResult, json.RawMessage(`{"workId":`))
	if err != nil {
		t.Fatalf("CallTool(accept_dispatch_result) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for invalid JSON decode")
	}
}

func TestBind_PlanDispatchValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolPlanDispatch,
		json.RawMessage(`{"dispatchId":"dispatch-1","correlationId":"corr-1","workIds":[],"workstationName":"ws","workerType":"coder","replayKey":"replay"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(plan_dispatch) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for validation failure")
	}
}

func TestBind_AcceptDispatchResultValidationFailureReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolAcceptDispatchResult,
		json.RawMessage(`{"dispatchId":"dispatch-1","correlationId":"corr-1","workId":"work-1","resultOutcome":"UNKNOWN"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(accept_dispatch_result) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake runtime root was invoked for validation failure")
	}
}

func TestBind_ControlPauseNotRunningReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRuntimeRoot{
		controlPause: func(_ context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolControlPause, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.runtime.not_running",
		true,
	)
	if envelope.Message != "factory runtime is not running" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "factory runtime is not running", envelope)
	}
	if envelope.Details == nil || envelope.Details["reason"] != "NOT_RUNNING" {
		t.Fatalf("error.details = %#v, want reason=NOT_RUNNING", envelope.Details)
	}
}

func TestBind_PlanDispatchNotFoundReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRuntimeRoot{
		planDispatch: func(_ context.Context, _ factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
			return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotFound
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolPlanDispatch,
		json.RawMessage(`{
			"dispatchId":"dispatch-1",
			"correlationId":"corr-1",
			"workIds":["work-1"],
			"workstationName":"ws-alpha",
			"workerType":"coder",
			"replayKey":"replay-1"
		}`),
	)
	if err != nil {
		t.Fatalf("CallTool(plan_dispatch) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.target.not_found",
		false,
	)
	if envelope.Message != "factory runtime target not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "factory runtime target not found", envelope)
	}
	if envelope.Details == nil || envelope.Details["reason"] != "NOT_FOUND" {
		t.Fatalf("error.details = %#v, want reason=NOT_FOUND", envelope.Details)
	}
}

func TestBind_ObserveInvalidObservationScopeFromRootReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRuntimeRoot{
		observe: func(_ context.Context, request factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			if request.Scope != factoryruntime.ObservationScopeProgress {
				t.Fatalf("scope = %q, want PROGRESS", request.Scope)
			}
			return factoryruntime.ObserveResult{}, factoryruntime.ErrInvalidObservationScope
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(
		context.Background(),
		factoryrunmcp.ToolObserve,
		json.RawMessage(`{"scope":"PROGRESS"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.observation.invalid_scope",
		false,
	)
	if envelope.Message != "factory runtime invalid observation scope" {
		t.Fatalf(
			"error.message = %q, want %q; envelope = %#v",
			envelope.Message,
			"factory runtime invalid observation scope",
			envelope,
		)
	}
	if envelope.Details == nil || envelope.Details["reason"] != "INVALID_OBSERVATION_SCOPE" {
		t.Fatalf("error.details = %#v, want reason=INVALID_OBSERVATION_SCOPE", envelope.Details)
	}
}

func TestBind_UnmappedRootErrorDoesNotLeakInternalPackagePaths(t *testing.T) {
	t.Parallel()

	fake := fakeRuntimeRoot{
		controlPause: func(_ context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			return factoryruntime.PauseResult{}, fmt.Errorf(
				"%s: connection reset\ngoroutine 1 [running]:\nmain.main()",
				internalLeakProbePath,
			)
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolControlPause, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) transport error = %v, want typed tool response", err)
	}
	assertEnvelopeDoesNotLeakInternalPaths(t, raw)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.execution.internal",
		false,
	)
	if envelope.Message != "factory runtime execution failed" {
		t.Fatalf("error.message = %q, want sanitized internal message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ControlPauseContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{
		Runtime: fakeRuntimeRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(ctx, factoryrunmcp.ToolControlPause, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake runtime root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.request.canceled",
		false,
	)
	if envelope.Message != "factory runtime request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ObserveContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeRuntimeRoot{
		observe: func(ctx context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return factoryruntime.ObserveResult{}, ctx.Err()
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			factoryrunmcp.ToolObserve,
			json.RawMessage(`{"scope":"PROGRESS"}`),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake runtime root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(observe) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.request.canceled",
		false,
	)
	if envelope.Message != "factory runtime request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_PlanDispatchContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRuntimeRoot{
		planDispatch: func(ctx context.Context, _ factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
			<-ctx.Done()
			return factoryruntime.PlanDispatchResult{}, ctx.Err()
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		factoryrunmcp.ToolPlanDispatch,
		json.RawMessage(`{
			"dispatchId":"dispatch-1",
			"correlationId":"corr-1",
			"workIds":["work-1"],
			"workstationName":"ws-alpha",
			"workerType":"coder",
			"replayKey":"replay-1"
		}`),
	)
	if err != nil {
		t.Fatalf("CallTool(plan_dispatch) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"factory_runtime.request.timed_out",
		true,
	)
	if envelope.Message != "factory runtime request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
}

func assertEnvelopeDoesNotLeakInternalPaths(t *testing.T, raw json.RawMessage) {
	t.Helper()
	if strings.Contains(string(raw), internalLeakProbePath) {
		t.Fatalf("tool response leaks internal package path: %s", raw)
	}
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
) *factoryrunmcp.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                `json:"result"`
		Error  *factoryrunmcp.ToolErrorEnvelope `json:"error"`
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
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}
