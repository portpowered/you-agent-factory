package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

type builderComponent struct{}

func (*builderComponent) Start(context.Context) error { return nil }
func (*builderComponent) Stop(context.Context) error  { return nil }
func (*builderComponent) Wait(context.Context) error  { return nil }

func builderPlan(close func() error) lifecycle.Plan {
	plan := lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "primary", Component: &builderComponent{}, Primary: true,
	}}}
	if close != nil {
		plan.Resources = []lifecycle.NamedResource{{Name: "owned", Resource: lifecycle.CloserFunc(close)}}
	}
	return plan
}

func TestLifecycleRunnerBuilderConsumesPlan(t *testing.T) {
	build, err := NewLifecycleRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	runner, err := build(t.Context(), builderPlan(nil), runtimeartifact.Diagnostics{}, nil)
	if err != nil || runner == nil {
		t.Fatalf("build runner = %T, %v", runner, err)
	}
}

func TestLifecycleRunnerBuilderCleansTypedNilComponentOnce(t *testing.T) {
	build, err := NewLifecycleRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	closeCalls := 0
	var component *builderComponent
	plan := lifecycle.Plan{
		Components: []lifecycle.NamedComponent{{Name: "primary", Component: component, Primary: true}},
		Resources:  []lifecycle.NamedResource{{Name: "owned", Resource: lifecycle.CloserFunc(func() error { closeCalls++; return nil })}},
	}
	_, err = build(t.Context(), plan, runtimeartifact.Diagnostics{}, nil)
	if err == nil || !strings.Contains(err.Error(), "primary") || closeCalls != 1 {
		t.Fatalf("error = %v, cleanup calls = %d", err, closeCalls)
	}
}

func TestLifecycleRunnerBuilderCleansCanceledPlan(t *testing.T) {
	build, err := NewLifecycleRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	closeCalls := 0
	_, err = build(ctx, builderPlan(func() error { closeCalls++; return nil }), runtimeartifact.Diagnostics{}, nil)
	if !errors.Is(err, context.Canceled) || closeCalls != 1 {
		t.Fatalf("error = %v, cleanup calls = %d", err, closeCalls)
	}
}

func TestLifecycleRunnerBuilderCleansPlanOnFactoryFailure(t *testing.T) {
	closeCalls := 0
	build, err := NewLifecycleRunnerBuilder(func(lifecycle.Plan, runtimeartifact.Diagnostics) (*runtimeapplication.ManagedRunner, error) {
		return nil, errors.New("factory failed")
	})
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	_, err = build(t.Context(), builderPlan(func() error { closeCalls++; return nil }), runtimeartifact.Diagnostics{}, nil)
	if err == nil || !strings.Contains(err.Error(), "factory failed") || closeCalls != 1 {
		t.Fatalf("error = %v, cleanup calls = %d", err, closeCalls)
	}
}
