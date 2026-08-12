package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRuntimeRootActivationRejectsIncompleteRequestBeforeStarting(t *testing.T) {
	t.Parallel()

	started := false
	root := newRuntimeRoot(t, func(context.Context, factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		started = true
		return &factoryruntime.RuntimeActivation{Service: newFoldHostedRuntimeStub("RUNNING")}, nil
	})

	_, err := root.Activate(context.Background(), factoryruntime.RuntimeActivationRequest{})
	if !errors.Is(err, factoryruntime.ErrRuntimeActivationMissingParameters) {
		t.Fatalf("Activate(incomplete) error = %v, want missing parameters", err)
	}
	if started {
		t.Fatal("activation operation ran for an incomplete request")
	}
}

func TestRuntimeRootActivationPublishesOnlyDetachedSuccessfulState(t *testing.T) {
	t.Parallel()

	active := newFoldHostedRuntimeStub("RUNNING")
	var received factoryruntime.RuntimeActivationRequest
	root := newRuntimeRoot(t, func(_ context.Context, request factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		received = request
		request.Snapshot.EffectiveFactory.Name = "operation-local-mutation"
		return &factoryruntime.RuntimeActivation{Service: active}, nil
	})
	request := foldRuntimeActivationRequest()

	result, err := root.Activate(context.Background(), request)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if result.RuntimeID != request.RuntimeID || result.State != factoryruntime.RuntimeLifecycleStateActive {
		t.Fatalf("Activate() result = %#v, want active %q", result, request.RuntimeID)
	}
	if request.Snapshot.EffectiveFactory.Name != "fold" {
		t.Fatalf("caller snapshot changed by activation operation: %q", request.Snapshot.EffectiveFactory.Name)
	}
	if received.Snapshot.EffectiveFactory.Name != "fold" {
		t.Fatalf("activation operation received aliased snapshot: %q", received.Snapshot.EffectiveFactory.Name)
	}

	observed, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus})
	if err != nil {
		t.Fatalf("Observe(after activation) error = %v", err)
	}
	if observed.Observation.Health.FactoryState != "RUNNING" {
		t.Fatalf("published delegate state = %q, want RUNNING", observed.Observation.Health.FactoryState)
	}
}

func TestRuntimeRootActivationRejectsDuplicateAndConflictingIdentity(t *testing.T) {
	t.Parallel()

	starts := 0
	active := newFoldHostedRuntimeStub("RUNNING")
	root := newRuntimeRoot(t, func(_ context.Context, _ factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		starts++
		return &factoryruntime.RuntimeActivation{Service: active}, nil
	})
	request := foldRuntimeActivationRequest()
	if _, err := root.Activate(context.Background(), request); err != nil {
		t.Fatalf("Activate(first) error = %v", err)
	}

	if _, err := root.Activate(context.Background(), foldRuntimeActivationRequest()); !errors.Is(err, factoryruntime.ErrRuntimeAlreadyActive) {
		t.Fatalf("Activate(duplicate) error = %v, want already active", err)
	}
	conflict := foldRuntimeActivationRequest()
	conflict.RuntimeID = "fold-runtime-conflict"
	if _, err := root.Activate(context.Background(), conflict); !errors.Is(err, factoryruntime.ErrRuntimeActivationConflict) {
		t.Fatalf("Activate(conflict) error = %v, want conflict", err)
	}
	if starts != 1 {
		t.Fatalf("activation operation calls = %d, want one successful start", starts)
	}

	observed, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus})
	if err != nil {
		t.Fatalf("Observe(after rejected activation) error = %v", err)
	}
	if observed.Observation.Health.FactoryState != "RUNNING" {
		t.Fatalf("active delegate changed after rejected activation: %q", observed.Observation.Health.FactoryState)
	}
}

func TestRuntimeRootActivationUnwindsFailedStartAndCanRetry(t *testing.T) {
	t.Parallel()

	started := 0
	cleaned := 0
	fail := true
	root := newRuntimeRoot(t, func(_ context.Context, _ factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		started++
		activation := &factoryruntime.RuntimeActivation{
			Close: func(context.Context) error {
				cleaned++
				return nil
			},
		}
		if fail {
			return activation, errors.New("worker pool failed to initialize")
		}
		activation.Service = newFoldHostedRuntimeStub("RUNNING")
		return activation, nil
	})

	_, err := root.Activate(context.Background(), foldRuntimeActivationRequest())
	if !errors.Is(err, factoryruntime.ErrRuntimeActivationFailed) {
		t.Fatalf("Activate(failed) error = %v, want activation failure", err)
	}
	if started != 1 || cleaned != 1 {
		t.Fatalf("failed activation lifecycle = started %d, cleaned %d; want 1/1", started, cleaned)
	}
	if _, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(after failed activation) error = %v, want ErrNotRunning", err)
	}

	fail = false
	if _, err := root.Activate(context.Background(), foldRuntimeActivationRequest()); err != nil {
		t.Fatalf("Activate(retry) error = %v", err)
	}
}

func TestRuntimeRootFailedCleanupRemainsExplicitlyRetryable(t *testing.T) {
	t.Parallel()

	cleanupCalls := 0
	cleanupFailure := true
	root := newRuntimeRoot(t, func(_ context.Context, _ factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		return &factoryruntime.RuntimeActivation{
			Close: func(context.Context) error {
				cleanupCalls++
				if cleanupFailure {
					return errors.New("runtime artifact is still open")
				}
				return nil
			},
		}, errors.New("runtime worker initialization failed")
	})
	request := foldRuntimeActivationRequest()

	if _, err := root.Activate(context.Background(), request); !errors.Is(err, factoryruntime.ErrRuntimeActivationFailed) {
		t.Fatalf("Activate(failed cleanup) error = %v, want activation failure", err)
	}
	if _, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(failed cleanup) error = %v, want ErrNotRunning", err)
	}
	if _, err := root.Activate(context.Background(), request); !errors.Is(err, factoryruntime.ErrRuntimeActivationConflict) {
		t.Fatalf("Activate(with pending cleanup) error = %v, want conflict", err)
	}

	if _, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); !errors.Is(err, factoryruntime.ErrRuntimeDeactivationFailed) {
		t.Fatalf("Deactivate(first cleanup retry) error = %v, want deactivation failure", err)
	}
	cleanupFailure = false
	result, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID})
	if err != nil {
		t.Fatalf("Deactivate(second cleanup retry) error = %v", err)
	}
	if result.State != factoryruntime.RuntimeLifecycleStateStopped || cleanupCalls != 3 {
		t.Fatalf("Deactivate(second cleanup retry) = %#v, cleanup calls %d; want STOPPED and 3", result, cleanupCalls)
	}
}

func TestRuntimeRootDeactivationRetainsStateUntilCleanupSucceeds(t *testing.T) {
	t.Parallel()

	cleanupCalls := 0
	cleanupFailure := true
	active := newFoldHostedRuntimeStub("RUNNING")
	root := newRuntimeRoot(t, func(_ context.Context, _ factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		return &factoryruntime.RuntimeActivation{
			Service: active,
			Close: func(context.Context) error {
				cleanupCalls++
				if cleanupFailure {
					return errors.New("sidecar still draining")
				}
				return nil
			},
		}, nil
	})
	request := foldRuntimeActivationRequest()
	if _, err := root.Activate(context.Background(), request); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	_, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID})
	if !errors.Is(err, factoryruntime.ErrRuntimeDeactivationFailed) {
		t.Fatalf("Deactivate(first) error = %v, want cleanup failure", err)
	}
	if _, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); err != nil {
		t.Fatalf("Observe(after failed deactivation) error = %v, want active delegate", err)
	}

	cleanupFailure = false
	result, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID})
	if err != nil {
		t.Fatalf("Deactivate(retry) error = %v", err)
	}
	if result.State != factoryruntime.RuntimeLifecycleStateStopped || cleanupCalls != 2 {
		t.Fatalf("Deactivate(retry) = %#v, cleanup calls %d; want STOPPED and 2", result, cleanupCalls)
	}
	if _, err := root.Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(after deactivation) error = %v, want ErrNotRunning", err)
	}
}

func newRuntimeRoot(
	t *testing.T,
	activation factoryruntime.RuntimeActivationOperation,
) factoryruntime.Root {
	t.Helper()
	root, err := factoryruntimewire.NewService(
		func() string { return "runtime-activation-test-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
		activation,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewService() returned nil root")
	}
	return root
}
