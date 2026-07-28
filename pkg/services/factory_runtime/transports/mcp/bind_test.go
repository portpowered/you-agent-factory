package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryrunmcp "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/mcp"
)

func TestBind_FakeRuntimeRootInvokedThroughCanonicalControlPauseTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		controlPause: func(_ context.Context, _ factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
			return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolControlPause, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(control_pause) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}
	if !strings.Contains(string(raw), `"Outcome":"ACCEPTED"`) {
		t.Fatalf("CallTool(control_pause) = %s, want accepted pause result", raw)
	}
}

func TestBind_FakeRuntimeRootInvokedThroughCanonicalObserveTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRuntimeRoot{
		invoked: &invoked,
		observe: func(_ context.Context, request factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			if request.Scope != factoryruntime.ObservationScopeStatus {
				t.Fatalf("scope = %q, want STATUS", request.Scope)
			}
			return factoryruntime.ObserveResult{
				Observation: factoryruntime.Observation{Status: factoryruntime.ObservationStatusActive},
			}, nil
		},
	}
	operation := factoryrunmcp.Bind(factoryrunmcp.RootDependencies{Runtime: fake})
	raw, err := operation(context.Background(), factoryrunmcp.ToolObserve, json.RawMessage(`{"scope":"STATUS"}`))
	if err != nil {
		t.Fatalf("CallTool(observe) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake runtime root was not invoked")
	}
	if !strings.Contains(string(raw), `"Status":"ACTIVE"`) {
		t.Fatalf("CallTool(observe) = %s, want active observation", raw)
	}
}

type fakeRuntimeRoot struct {
	factoryruntime.Service
	invoked              *bool
	controlPause         func(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error)
	observe              func(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
	planDispatch         func(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error)
	acceptDispatchResult func(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error)
}

func (root fakeRuntimeRoot) markInvoked() {
	if root.invoked != nil {
		*root.invoked = true
	}
}

func (root fakeRuntimeRoot) ControlPause(
	ctx context.Context,
	request factoryruntime.PauseRequest,
) (factoryruntime.PauseResult, error) {
	root.markInvoked()
	if root.controlPause == nil {
		panic("unexpected ControlPause on fake runtime root")
	}
	return root.controlPause(ctx, request)
}

func (root fakeRuntimeRoot) Observe(
	ctx context.Context,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	root.markInvoked()
	if root.observe == nil {
		panic("unexpected Observe on fake runtime root")
	}
	return root.observe(ctx, request)
}

func (root fakeRuntimeRoot) PlanDispatch(
	ctx context.Context,
	request factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	root.markInvoked()
	if root.planDispatch == nil {
		panic("unexpected PlanDispatch on fake runtime root")
	}
	return root.planDispatch(ctx, request)
}

func (root fakeRuntimeRoot) AcceptDispatchResult(
	ctx context.Context,
	request factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	root.markInvoked()
	if root.acceptDispatchResult == nil {
		panic("unexpected AcceptDispatchResult on fake runtime root")
	}
	return root.acceptDispatchResult(ctx, request)
}
