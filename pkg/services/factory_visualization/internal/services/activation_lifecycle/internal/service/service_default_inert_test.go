package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
)

func TestNewRejectsMissingActivationLifecycleDependencies(t *testing.T) {
	t.Parallel()

	clock := fixedLifecycleClock{now: time.Unix(1, 0)}
	sink := lifecycleSinkFunc(func(activationlifecycle.View) {})
	projections := lifecycleProjectionStub{}
	source := &lifecycleSourceStub{}
	tests := []struct {
		name string
		new  func() (*lifecycleservice.Service, error)
		want string
	}{
		{"source", func() (*lifecycleservice.Service, error) {
			return lifecycleservice.New(nil, projections, clock, sink, nil)
		}, "event source"},
		{"projections", func() (*lifecycleservice.Service, error) {
			return lifecycleservice.New(source, nil, clock, sink, nil)
		}, "projection service"},
		{"clock", func() (*lifecycleservice.Service, error) {
			return lifecycleservice.New(source, projections, nil, sink, nil)
		}, "clock"},
		{"sink", func() (*lifecycleservice.Service, error) {
			return lifecycleservice.New(source, projections, clock, nil, nil)
		}, "presentation sink"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, err := test.new()
			if service != nil {
				t.Fatal("New() returned a usable activation lifecycle owner without required collaborators")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestActivationLifecycleDefaultInertConstruction(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	source := &lifecycleSourceStub{
		stream: newLifecycleEventStream(),
	}
	source.subscribeHook = func() { subscribeCalls++ }
	owner, err := lifecycleservice.New(
		source,
		lifecycleProjectionStub{},
		fixedLifecycleClock{now: time.Unix(1, 0)},
		lifecycleSinkFunc(func(activationlifecycle.View) { presentCalls++ }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "construction")

	_, err = owner.Join(context.Background(), activationlifecycle.JoinRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorNotActivated, "Join before Activate")
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "Join before Activate")

	_, err = owner.Activate(context.Background(), activationlifecycle.ActivateRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorMissingParameters, "Activate missing parameters")
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "missing-parameter Activate")

	_, err = owner.Activate(context.Background(), activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateMode("UNSUPPORTED"),
	})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorMissingParameters, "Activate unsupported mode")
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "unsupported-mode Activate")
}

func assertActivationLifecycleInert(t *testing.T, subscribeCalls, presentCalls int, label string) {
	t.Helper()
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatalf("%s: subscribe=%d present=%d, want no live subscription or presentation side effects", label, subscribeCalls, presentCalls)
	}
}
