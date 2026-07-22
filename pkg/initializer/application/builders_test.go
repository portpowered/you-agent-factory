package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

type builderComponent struct{}

func (*builderComponent) Start(context.Context) error { return nil }
func (*builderComponent) Stop(context.Context) error  { return nil }
func (*builderComponent) Wait(context.Context) error  { return nil }

type builderContextKey struct{}

func builderPlan(close func() error) lifecycle.Plan {
	plan := lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "primary", Component: &builderComponent{}, Primary: true,
	}}}
	if close != nil {
		plan.Resources = []lifecycle.NamedResource{{Name: "opened", Resource: lifecycle.CloserFunc(close)}}
	}
	return plan
}

func TestRuntimeRunnerBuilderConsumesNeutralLifecycleOpening(t *testing.T) {
	build := newTestRuntimeRunnerBuilder(t)
	plan := builderPlan(nil)
	runner, err := build(t.Context(), func(context.Context) (initializer.OpenedApplication, error) {
		return initializer.OpenedApplication{Plan: plan}, nil
	})
	if err != nil || runner == nil {
		t.Fatalf("build runner = %T, %v", runner, err)
	}
}

func TestRuntimeRunnerBuilderFailsClosedAndCleansTypedNilRuntimeOnce(t *testing.T) {
	build := newTestRuntimeRunnerBuilder(t)
	closeCalls := 0
	var component *builderComponent
	plan := lifecycle.Plan{
		Components: []lifecycle.NamedComponent{{Name: "primary", Component: component, Primary: true}},
		Resources:  []lifecycle.NamedResource{{Name: "opened", Resource: lifecycle.CloserFunc(func() error { closeCalls++; return nil })}},
	}
	_, err := build(t.Context(), func(context.Context) (initializer.OpenedApplication, error) {
		return initializer.OpenedApplication{Plan: plan}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("typed nil component error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", closeCalls)
	}
}

func TestRuntimeRunnerBuilderRejectsCanceledContextBeforeOpening(t *testing.T) {
	build := newTestRuntimeRunnerBuilder(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	opened := false
	_, err := build(ctx, func(context.Context) (initializer.OpenedApplication, error) {
		opened = true
		return initializer.OpenedApplication{}, nil
	})
	if !errors.Is(err, context.Canceled) || opened {
		t.Fatalf("error = %v, opened = %t", err, opened)
	}
}

func TestRuntimeRunnerBuilderCleansOpenedResourceWhenResourceFactoryReturnsNil(t *testing.T) {
	closeCalls := 0
	build, err := NewRuntimeRunnerBuilder(func(lifecycle.Plan, runtimeartifact.Diagnostics) (*runtimeapplication.ManagedRunner, error) {
		return nil, errors.New("factory failed")
	})
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	_, err = build(t.Context(), func(context.Context) (initializer.OpenedApplication, error) {
		return initializer.OpenedApplication{Plan: builderPlan(func() error { closeCalls++; return nil })}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "factory failed") {
		t.Fatalf("error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", closeCalls)
	}
}

func TestOpenedStdioRunnerBuilderCleansResourcesWhenSessionOpeningFails(t *testing.T) {
	build, err := NewOpenedStdioRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	openingErr := errors.New("opening failed")
	_, err = build(t.Context(), initializer.OpenedStdioApplication{OpenSession: func(context.Context, io.Reader, io.Writer) (initializer.OpenedApplication, error) {
		return initializer.OpenedApplication{}, openingErr
	}}, strings.NewReader(""), io.Discard)
	if !errors.Is(err, openingErr) {
		t.Fatalf("opening error = %v", err)
	}
}

func TestStdioRunnerBuilderPassesExactContextToNeutralOpening(t *testing.T) {
	build, err := NewStdioRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	ctx := context.WithValue(t.Context(), builderContextKey{}, "present")
	seen := false
	runner, err := build(ctx, func(openCtx context.Context, _ io.Reader, _ io.Writer) (initializer.OpenedApplication, error) {
		seen = openCtx.Value(builderContextKey{}) == "present"
		return initializer.OpenedApplication{Plan: builderPlan(nil)}, nil
	}, strings.NewReader(""), io.Discard)
	if err != nil || runner == nil || !seen {
		t.Fatalf("runner = %T, context = %t, error = %v", runner, seen, err)
	}
}

func TestOpenedStdioRunnerBuilderRejectsTypedNilSessionAndCleansOnce(t *testing.T) {
	build, err := NewOpenedStdioRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct builder: %v", err)
	}
	closeCalls := 0
	_, err = build(t.Context(), initializer.OpenedStdioApplication{OpenSession: func(context.Context, io.Reader, io.Writer) (initializer.OpenedApplication, error) {
		var component *builderComponent
		return initializer.OpenedApplication{Plan: lifecycle.Plan{
			Components: []lifecycle.NamedComponent{{Name: "primary", Component: component, Primary: true}},
			Resources:  []lifecycle.NamedResource{{Name: "opened", Resource: lifecycle.CloserFunc(func() error { closeCalls++; return nil })}},
		}}, nil
	}}, strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("typed nil session error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", closeCalls)
	}
}

func newTestRuntimeRunnerBuilder(t *testing.T) initializer.RuntimeRunnerBuilder {
	t.Helper()
	build, err := NewRuntimeRunnerBuilder(runtimeapplication.NewManagedRunner)
	if err != nil {
		t.Fatalf("construct runtime runner builder: %v", err)
	}
	return build
}
