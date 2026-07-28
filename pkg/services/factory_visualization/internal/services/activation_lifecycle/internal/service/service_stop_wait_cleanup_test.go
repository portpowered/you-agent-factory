package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

func TestActivationLifecycleWaitBeforeActivateReturnsNotActivated(t *testing.T) {
	t.Parallel()

	owner := mustNewActivationLifecycleOwner(t, newLifecycleEventStream())
	err := owner.Wait(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("Wait before Activate: error = %v, want not-started failure", err)
	}

	_, err = owner.Join(context.Background(), activationlifecycle.JoinRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorNotActivated, "Join before Activate")
}

func TestActivationLifecycleStopDrainCancelsAndJoinsWithoutSecondStop(t *testing.T) {
	t.Parallel()

	stream := newLifecycleEventStream()
	subscribeCalls := 0
	source := &lifecycleSourceStub{
		stream:   stream,
		snapshot: &activationlifecycle.EngineObservation{TickCount: 1},
	}
	source.subscribeHook = func() { subscribeCalls++ }
	owner := mustNewActivationLifecycleOwnerWithSource(t, source)

	ctx := context.Background()
	result, err := owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate: error = %v", err)
	}
	if result.State != activationlifecycle.LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, activationlifecycle.LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", subscribeCalls)
	}

	stopResult, err := owner.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{})
	if err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
	if stopResult.State != activationlifecycle.LifecycleStateStopped {
		t.Fatalf("StopDrain state = %q, want %q", stopResult.State, activationlifecycle.LifecycleStateStopped)
	}
	if err := owner.Wait(context.Background()); err != nil {
		t.Fatalf("Wait after StopDrain: error = %v, want clean shutdown", err)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls after StopDrain = %d, want no reopened subscription", subscribeCalls)
	}
}

func TestActivationLifecycleWaitBlocksUntilSubscriptionExits(t *testing.T) {
	t.Parallel()

	owner := mustNewActivationLifecycleOwner(t, newLifecycleEventStream())
	ctx := context.Background()
	if _, err := owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("Activate: error = %v", err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- owner.Wait(ctx)
	}()

	select {
	case err := <-waitDone:
		t.Fatalf("Wait returned before StopDrain: error = %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := owner.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}

	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("Wait after subscription exit: error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not unblock after StopDrain")
	}
}

func TestActivationLifecycleWaitReturnsStreamRunError(t *testing.T) {
	t.Parallel()

	stream, events := newClosableLifecycleEventStream()
	owner := mustNewActivationLifecycleOwner(t, stream)
	ctx := context.Background()
	if _, err := owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("Activate: error = %v", err)
	}

	close(events)
	err := owner.Wait(ctx)
	if err == nil || !strings.Contains(err.Error(), "stream closed unexpectedly") {
		t.Fatalf("Wait after stream close: error = %v, want unexpected stream close", err)
	}
}

func TestActivationLifecycleRepeatedStopDrainIsIdempotent(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	source := &lifecycleSourceStub{
		stream:   newLifecycleEventStream(),
		snapshot: &activationlifecycle.EngineObservation{TickCount: 1},
	}
	source.subscribeHook = func() { subscribeCalls++ }
	owner := mustNewActivationLifecycleOwnerWithSource(t, source)

	ctx := context.Background()
	if _, err := owner.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("Activate: error = %v", err)
	}

	for i := 0; i < 2; i++ {
		stopResult, err := owner.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{})
		if err != nil {
			t.Fatalf("StopDrain iteration %d: error = %v", i, err)
		}
		if stopResult.State != activationlifecycle.LifecycleStateStopped {
			t.Fatalf("StopDrain iteration %d state = %q, want %q", i, stopResult.State, activationlifecycle.LifecycleStateStopped)
		}
	}
	for i := 0; i < 2; i++ {
		if err := owner.Stop(context.Background()); err != nil {
			t.Fatalf("Stop iteration %d: error = %v", i, err)
		}
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1 with no reopened subscription", subscribeCalls)
	}
}

func TestActivationLifecycleStartStopWaitCleanup(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	var subscribeMu sync.Mutex
	source := &lifecycleSourceStub{
		stream:   newLifecycleEventStream(),
		snapshot: &activationlifecycle.EngineObservation{TickCount: 1},
	}
	source.subscribeHook = func() {
		subscribeMu.Lock()
		subscribeCalls++
		subscribeMu.Unlock()
	}
	owner := mustNewActivationLifecycleOwnerWithSource(t, source)

	ctx := context.Background()
	if err := owner.Start(ctx); err != nil {
		t.Fatalf("Start: error = %v", err)
	}
	subscribeMu.Lock()
	startedSubscribes := subscribeCalls
	subscribeMu.Unlock()
	if startedSubscribes != 1 {
		t.Fatalf("subscribe calls after Start = %d, want 1", startedSubscribes)
	}

	if err := owner.Stop(ctx); err != nil {
		t.Fatalf("Stop: error = %v", err)
	}
	if err := owner.Wait(ctx); err != nil {
		t.Fatalf("Wait after Stop: error = %v", err)
	}
	subscribeMu.Lock()
	finalSubscribes := subscribeCalls
	subscribeMu.Unlock()
	if finalSubscribes != 1 {
		t.Fatalf("subscribe calls after cleanup = %d, want 1", finalSubscribes)
	}
}

func mustNewActivationLifecycleOwner(
	t *testing.T,
	stream *factorydefinitions.FactoryEventStream,
) *lifecycleservice.Service {
	t.Helper()
	source := &lifecycleSourceStub{
		stream:   stream,
		snapshot: &activationlifecycle.EngineObservation{TickCount: 1},
	}
	return mustNewActivationLifecycleOwnerWithSource(t, source)
}

func mustNewActivationLifecycleOwnerWithSource(
	t *testing.T,
	source *lifecycleSourceStub,
) *lifecycleservice.Service {
	t.Helper()
	owner, err := lifecycleservice.New(
		source,
		&recordingsstub.Service{},
		fixedLifecycleClock{now: time.Unix(1, 0)},
		lifecycleSinkFunc(func(activationlifecycle.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return owner
}
