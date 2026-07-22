package runtimeapplication

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

type lifecyclePlanComponentStub struct{}

func (*lifecyclePlanComponentStub) Start(context.Context) error { return nil }
func (*lifecyclePlanComponentStub) Stop(context.Context) error  { return nil }
func (*lifecyclePlanComponentStub) Wait(context.Context) error  { return nil }

func TestBuildLifecyclePlanUsesAlreadyBoundTransportComponent(t *testing.T) {
	transport := &lifecyclePlanComponentStub{}
	plan := lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "already-bound transport", Component: transport, Primary: true,
	}}}
	runner, err := NewManagedRunner(plan, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	if runner.plan.Components[0].Component != transport {
		t.Fatalf("transport component = %T, want exact bound transport", runner.plan.Components[0].Component)
	}
}

func TestBuildLifecyclePlanFailsClosedWithoutRuntimeOrTransport(t *testing.T) {
	_, err := NewManagedRunner(lifecycle.Plan{}, runtimeartifact.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "exactly one primary") {
		t.Fatalf("missing primary error = %v", err)
	}
}

func TestBuildLifecyclePlanFailsClosedWithTypedNilRuntime(t *testing.T) {
	var component *lifecyclePlanComponentStub
	_, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "primary", Component: component, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("typed nil component error = %v", err)
	}
}
