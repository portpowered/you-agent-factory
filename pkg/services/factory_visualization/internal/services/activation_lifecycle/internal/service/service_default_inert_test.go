package service_test

import (
	"context"
	"testing"
	"time"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

func TestActivationLifecycleDefaultInertConstruction(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	source := &lifecycleSourceStub{
		stream: newLifecycleEventStream(),
	}
	source.subscribeHook = func() { subscribeCalls++ }
	ownerBehavior := lifecycleservice.NewOwner(&recordingsstub.Service{})
	owner := ownerBehavior.Open(source, fixedLifecycleClock{now: time.Unix(1, 0)}, lifecycleSinkFunc(func(activationlifecycle.View) { presentCalls++ }), nil)
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "construction")

	_, err := owner.Join(context.Background(), activationlifecycle.JoinRequest{})
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
