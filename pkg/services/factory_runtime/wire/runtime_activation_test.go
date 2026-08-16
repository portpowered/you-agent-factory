package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

func TestRuntimeRootActivationDetachesExplicitInputs(t *testing.T) {
	t.Parallel()

	env := map[string]string{"TOKEN": "original"}
	request := foldRuntimeActivationRequest()
	request.Inputs = factoryruntime.RuntimeActivationInputs{
		Workers: factoryruntime.RuntimeActivationWorkerInputs{
			MockWorkers: &factoryruntime.RuntimeActivationMockWorkersConfig{
				MockWorkers: []factoryruntime.RuntimeActivationMockWorker{{
					ScriptConfig: &factoryruntime.RuntimeActivationMockScript{Env: env},
				}},
			},
		},
	}
	var received factoryruntime.RuntimeActivationRequest
	root := newRuntimeRoot(t, func(_ context.Context, got factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		received = got
		return &factoryruntime.RuntimeActivation{Service: newFoldHostedRuntimeStub("RUNNING")}, nil
	})
	if _, err := root.Activate(context.Background(), request); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	env["TOKEN"] = "caller-mutated"
	if got := received.Inputs.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Env["TOKEN"]; got != "original" {
		t.Fatalf("activation operation received aliased inputs: %q", got)
	}
}

func TestRuntimeRootActivationReturnsPublishedRuntimeHandoff(t *testing.T) {
	t.Parallel()

	active := newFoldHostedRuntimeStub("RUNNING")
	root := newRuntimeRoot(t, func(_ context.Context, request factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		return &factoryruntime.RuntimeActivation{
			Service: active,
		}, nil
	})
	request := foldRuntimeActivationRequest()

	result, err := root.Activate(context.Background(), request)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if result.Runtime.RuntimeID != request.RuntimeID {
		t.Fatalf("published handoff RuntimeID = %q, want %q", result.Runtime.RuntimeID, request.RuntimeID)
	}
	if result.Runtime.FactorySessionID != request.FactorySessionID {
		t.Fatalf("published handoff FactorySessionID = %q, want %q", result.Runtime.FactorySessionID, request.FactorySessionID)
	}
	if result.Runtime.Service != active {
		t.Fatal("published handoff did not return the activated Service")
	}
	if result.Binding.IsZero() || !result.Binding.Equal(result.Runtime.Binding) {
		t.Fatal("activation result did not publish the same opaque Runtime binding as its handoff")
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
	conflict.Snapshot.EffectiveFactory.Name = "different-factory"
	if _, err := root.Activate(context.Background(), conflict); !errors.Is(err, factoryruntime.ErrRuntimeActivationConflict) {
		t.Fatalf("Activate(conflicting identity) error = %v, want conflict", err)
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

func TestRuntimeRootActivatesDistinctBindingsInIsolation(t *testing.T) {
	t.Parallel()

	starts := make(map[string]int)
	active := make(map[string]*foldHostedRuntimeStub)
	root := newRuntimeRoot(t, func(_ context.Context, request factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		starts[request.RuntimeID]++
		runtime := newFoldHostedRuntimeStub(interfaces.FactoryState(request.RuntimeID))
		active[request.RuntimeID] = runtime
		return &factoryruntime.RuntimeActivation{Service: runtime}, nil
	})

	firstRequest := foldRuntimeActivationRequest()
	first, err := root.Activate(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("Activate(first) error = %v", err)
	}
	secondRequest := foldRuntimeActivationRequest()
	secondRequest.RuntimeID = "fold-runtime-2"
	secondRequest.FactorySessionID = "fold-session-2"
	second, err := root.Activate(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("Activate(second) error = %v", err)
	}
	if first.Binding.IsZero() || second.Binding.IsZero() {
		t.Fatal("successful activations must publish non-zero Runtime bindings")
	}
	if first.Binding.Equal(second.Binding) {
		t.Fatal("distinct Runtime identities published the same binding")
	}

	firstObserved, err := first.Binding.Service().Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus})
	if err != nil {
		t.Fatalf("first binding Observe() error = %v", err)
	}
	if got := firstObserved.Observation.Health.FactoryState; got != firstRequest.RuntimeID {
		t.Fatalf("first binding observed state %q, want %q", got, firstRequest.RuntimeID)
	}
	secondObserved, err := second.Binding.Service().Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus})
	if err != nil {
		t.Fatalf("second binding Observe() error = %v", err)
	}
	if got := secondObserved.Observation.Health.FactoryState; got != secondRequest.RuntimeID {
		t.Fatalf("second binding observed state %q, want %q", got, secondRequest.RuntimeID)
	}
	if starts[firstRequest.RuntimeID] != 1 || starts[secondRequest.RuntimeID] != 1 {
		t.Fatalf("activation calls = %#v, want one per identity", starts)
	}

	if _, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{RuntimeID: firstRequest.RuntimeID}); err != nil {
		t.Fatalf("Deactivate(first) error = %v", err)
	}
	if _, err := first.Binding.Service().Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("first binding after deactivation error = %v, want ErrNotRunning", err)
	}
	if _, err := second.Binding.Service().Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); err != nil {
		t.Fatalf("second binding after first deactivation error = %v", err)
	}
}

func TestRuntimeBindingOwnsRootDeactivation(t *testing.T) {
	t.Parallel()

	cleanupCalls := 0
	root := newRuntimeRoot(t, func(_ context.Context, _ factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		return &factoryruntime.RuntimeActivation{
			Service: newFoldHostedRuntimeStub(interfaces.FactoryState("bound")),
			Close: func(context.Context) error {
				cleanupCalls++
				return nil
			},
		}, nil
	})
	request := foldRuntimeActivationRequest()
	activated, err := root.Activate(context.Background(), request)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	deactivated, err := root.Deactivate(context.Background(), factoryruntime.RuntimeDeactivationRequest{Binding: activated.Binding})
	if err != nil {
		t.Fatalf("Deactivate(binding) error = %v", err)
	}
	if deactivated.RuntimeID != request.RuntimeID || deactivated.State != factoryruntime.RuntimeLifecycleStateStopped {
		t.Fatalf("Deactivate(binding) = %#v, want stopped %q", deactivated, request.RuntimeID)
	}
	if cleanupCalls != 1 {
		t.Fatalf("owned cleanup calls = %d, want one", cleanupCalls)
	}
	if _, err := activated.Binding.Service().Observe(context.Background(), factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus}); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("bound service after binding deactivation error = %v, want ErrNotRunning", err)
	}
	if _, err := activated.Binding.Deactivate(context.Background()); !errors.Is(err, factoryruntime.ErrRuntimeNotActive) {
		t.Fatalf("repeated binding Deactivate() error = %v, want ErrRuntimeNotActive", err)
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
) interface {
	factoryruntime.Service
	Activate(context.Context, factoryruntime.RuntimeActivationRequest) (factoryruntime.RuntimeActivationResult, error)
	Deactivate(context.Context, factoryruntime.RuntimeDeactivationRequest) (factoryruntime.RuntimeDeactivationResult, error)
} {
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
