package service_test

import (
	"context"
	"testing"
	"time"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
)

func TestActivationLifecycleExplicitRequestActivation(t *testing.T) {
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

	_, err = owner.Activate(context.Background(), activationlifecycle.ActivateRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorMissingParameters, "zero-value Activate")
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "zero-value Activate")

	_, err = owner.Activate(context.Background(), activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateMode("UNSUPPORTED"),
	})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorMissingParameters, "unsupported Activate mode")
	assertActivationLifecycleInert(t, subscribeCalls, presentCalls, "unsupported Activate mode")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate RETAINED_THEN_LIVE: error = %v", err)
	}
	if result.State != activationlifecycle.LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, activationlifecycle.LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1 after explicit Activate", subscribeCalls)
	}

	_, err = owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorAlreadyActivated, "repeat Activate")

	cancel()
	if _, err := owner.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
}
