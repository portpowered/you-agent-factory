package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryrunmcp "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/mcp"
)

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

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
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
