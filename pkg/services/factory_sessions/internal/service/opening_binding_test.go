package service

import (
	"context"
	"errors"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The final operation receives controlled acquired handles; no graph is assembled.
func bindOpeningStateFixture(t *testing.T, ctx context.Context, gateway roles.SessionGateway,
	runtime factoryruntime.Service, scope models.RuntimeScopeRef, startup runtimeports.RuntimeInstance,
	lifecycle roles.LifecycleRuntime, process roles.ProcessRuntime, directory, runtimeID, backendID string,
	closeResources func() error, ids ...string) (*runtimebinding.SessionState, func() error) {
	t.Helper()
	id := ""
	if len(ids) > 0 {
		id = ids[0]
	}
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(closeResources)
	var session roles.ApplicationRuntime = bindingSessionRuntime{ApplicationRuntime: lifecycle, Service: runtime}
	if selected, ok := runtime.(roles.ApplicationRuntime); ok {
		session = selected
	}
	opened := &runtimebinding.SessionState{ProjectionBackendScope: backendID}
	err := NewRuntimeOpeningBinding(bindingIdentityReader(gateway), nil, nil, nil).Bind(ctx,
		RuntimeOpeningBindingRequest{Facts: roles.SessionOpeningFacts{
			FactorySessionID: id, RuntimeID: runtimeID, BackendScopeID: backendID,
			Directory: directory, ModelsScope: scope,
		}}, opened, openingCoordinatorClock{}, startup, session, process,
		&factoryruntime.RuntimeActivation{Service: runtime}, &bindingExecution{}, func(context.Context) error { return nil }, cleanup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup.Close(); err != nil {
			t.Error(err)
		}
	})
	return opened, cleanup.Close
}

type bindingSessionRuntime struct {
	roles.ApplicationRuntime
	factoryruntime.Service
}

type bindingExecution struct {
	durableexecution.Service
	bind func(string, string, string, providers.Service, platformprocess.CommandRunner, workers.ProgressPublisher,
		func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)) (func(), error)
}

func (execution *bindingExecution) BindWorkerScope(id string, _ factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID, generation string, provider providers.Service, _ *workers.MockWorkersConfig,
	runner platformprocess.CommandRunner, progress workers.ProgressPublisher,
	starter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) (func(), error) {
	if execution.bind != nil {
		return execution.bind(id, runtimeID, generation, provider, runner, progress, starter)
	}
	return func() {}, nil
}

type bindingObservation struct {
	workersessions.ObservationService
}
type bindingScopedRuntime struct {
	roles.ApplicationRuntime
	factoryruntime.Service
	observation workersessions.ObservationService
	selected    string
}

func (runtime *bindingScopedRuntime) WorkerSessionsObservationForSession(id string) workersessions.ObservationService {
	runtime.selected = id
	return runtime.observation
}

type bindingLegacyRuntime struct {
	factoryruntime.Service
	observation workersessions.ObservationService
}

func (runtime bindingLegacyRuntime) WorkerSessionsObservation() workersessions.ObservationService {
	return runtime.observation
}

func TestRuntimeOpeningBindingObservationCapabilities(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"startup scoped", "startup legacy", "root scoped", "absent"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			selected, current := &bindingObservation{}, &bindingObservation{}
			root := &bindingScopedRuntime{observation: current}
			startupScope := &bindingScopedRuntime{observation: selected}
			startup := &portableReplayRuntimeRecord{}
			var runtime factoryruntime.Service = root
			want := workersessions.ObservationService(selected)
			switch name {
			case "startup scoped":
				startup.service = startupScope
			case "startup legacy":
				startup.service = bindingLegacyRuntime{observation: selected}
			case "root scoped":
				want = current
			case "absent":
				runtime = &portableReplayRuntimeService{}
				want = nil
			}
			gateway := &runtimeProductsSessionsRole{readLiveSession: func(id string) (factorysessions.LiveControlSnapshot, error) {
				if id != "explicit" {
					t.Fatalf("lookup selected %q", id)
				}
				return factorysessions.LiveControlSnapshot{Context: factorysessions.ProjectionContext{FactorySessionID: "effective"}}, nil
			}}
			opened, _ := bindOpeningStateFixture(t, t.Context(), gateway, runtime, models.RuntimeScopeRef{}, startup,
				nil, nil, "/selected", "runtime", "backend", nil, "explicit")
			if opened.WorkerSessionsObservation() != want || opened.ModelInvocation.FactorySessionID != "effective" {
				t.Fatalf("observation selection = %v, model ID = %q", opened.WorkerSessionsObservation(), opened.ModelInvocation.FactorySessionID)
			}
			if name == "startup scoped" && (startupScope.selected != "effective" || root.selected != "") {
				t.Fatalf("startup/root selection = %q/%q", startupScope.selected, root.selected)
			}
			if name == "root scoped" && root.selected != "effective" {
				t.Fatal("lost scoped legacy handle")
			}
		})
	}
}

func TestRuntimeOpeningBindingFailureAndSameIdentityRetry(t *testing.T) {
	t.Parallel()
	cause := &factorysessions.DetachedRequestError{Field: "binding", Message: "controlled"}
	cleanupCause := errors.New("controlled close failure")
	cleanup := &runtimeOpeningCleanup{}
	closes := 0
	cleanup.Add(func() error {
		closes++
		if closes == 1 {
			return cleanupCause
		}
		return nil
	})
	lookups := 0
	gateway := &runtimeProductsSessionsRole{readLiveSession: func(string) (factorysessions.LiveControlSnapshot, error) {
		lookups++
		return factorysessions.LiveControlSnapshot{}, nil
	}}
	operation := NewRuntimeOpeningBinding(bindingIdentityReader(gateway), nil, nil, nil)
	clock := openingCoordinatorClock{}
	session := bindingSessionRuntime{}
	request := RuntimeOpeningBindingRequest{Facts: roles.SessionOpeningFacts{FactorySessionID: "candidate", RuntimeID: "runtime-candidate"}}
	execution := &bindingExecution{bind: func(id, runtimeID, generation string, _ providers.Service, _ platformprocess.CommandRunner,
		_ workers.ProgressPublisher, _ func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)) (func(), error) {
		if id != "candidate" || runtimeID != "runtime-candidate" {
			t.Fatal("retargeted failure")
		}
		return nil, cause
	}}
	opened := &runtimebinding.SessionState{}
	err := operation.Bind(t.Context(), request, opened, clock, inertHostedInstance{}, session, nil,
		&factoryruntime.RuntimeActivation{}, execution, nil, cleanup)
	assertBindingTypedFailure(t, opened, err, cause, lookups)
	if err := cleanup.Close(); !errors.Is(err, cleanupCause) {
		t.Fatalf("close = %v", err)
	}
	if err := cleanup.Close(); err != nil || closes != 2 {
		t.Fatalf("retry close = %v, calls=%d", err, closes)
	}
	releases := 0
	execution.bind = func(string, string, string, providers.Service, platformprocess.CommandRunner, workers.ProgressPublisher,
		func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)) (func(), error) {
		return func() { releases++ }, nil
	}
	err = operation.Bind(t.Context(), request, opened, clock, inertHostedInstance{}, session, nil,
		&factoryruntime.RuntimeActivation{}, execution, nil, cleanup)
	if err != nil || opened.Clock != clock || opened.ModelInvocation.FactorySessionID != "candidate" {
		t.Fatalf("corrected retry = %+v, %v", opened, err)
	}
	for range 2 {
		if err := cleanup.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if releases != 1 || closes != 2 {
		t.Fatalf("release/close calls=%d/%d", releases, closes)
	}
}

func TestRuntimeOpeningBindingSelectedEffectsAndPeerRelease(t *testing.T) {
	t.Parallel()
	runner := &executionBindingRunner{}
	provider := &bindingProvider{}
	operation := NewRuntimeOpeningBinding(nil, nil, provider, runner)
	if runner.calls != 0 {
		t.Fatal("constructor invoked command effect")
	}
	active := map[string]string{}
	progress := map[string]string{}
	cleanups := map[string]*runtimeOpeningCleanup{}
	session := bindingSessionRuntime{}
	for _, id := range []string{"candidate", "peer", "candidate"} {
		generation := "generation-" + id
		key := id
		if active[id] != "" {
			generation += "-new"
			key += "-new"
		}
		cleanup := &runtimeOpeningCleanup{}
		cleanups[key] = cleanup
		runtime := &portableReplayRuntimeRecord{generation: generation,
			progress: func(fragment workers.ProgressFragment) { progress[key] = fragment.Payload }}
		execution := &bindingExecution{bind: func(gotID, runtimeID, gotGeneration string, gotProvider providers.Service,
			gotRunner platformprocess.CommandRunner, publish workers.ProgressPublisher,
			starter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)) (func(), error) {
			assertBindingChildEffects(t, id, key, generation, provider, runner,
				gotID, runtimeID, gotGeneration, gotProvider, gotRunner, publish, starter)
			active[id] = generation
			return func() {
				if active[id] == generation {
					delete(active, id)
				}
			}, nil
		}}
		opened := &runtimebinding.SessionState{}
		err := operation.Bind(t.Context(), RuntimeOpeningBindingRequest{Facts: roles.SessionOpeningFacts{
			FactorySessionID: id, RuntimeID: "runtime-" + id,
		}}, opened, openingCoordinatorClock{}, runtime, session, nil, &factoryruntime.RuntimeActivation{}, execution, nil, cleanup)
		if err != nil || opened.ModelInvocation.GenerationID != generation || progress[key] != key {
			t.Fatalf("scoped result/progress = %+v, %q, %v", opened.ModelInvocation, progress[key], err)
		}
	}
	closeBindingFixture(t, cleanups["candidate"])
	closeBindingFixture(t, cleanups["candidate"])
	if active["peer"] != "generation-peer" || active["candidate"] != "generation-candidate-new" {
		t.Fatalf("stale/repeated release cleared peer or replacement: %v", active)
	}
	closeBindingFixture(t, cleanups["candidate-new"])
	closeBindingFixture(t, cleanups["peer"])
	if len(active) != 0 || runner.calls != 3 {
		t.Fatalf("owned releases/effects = %v/%d", active, runner.calls)
	}
}

type bindingProvider struct{ providers.Service }

func TestRuntimeOpeningBindingFailurePreservesSelectedState(t *testing.T) {
	t.Parallel()
	cause := &factorysessions.DetachedRequestError{Field: "binding", Message: "controlled"}
	clock := openingCoordinatorClock{}
	selected := &runtimebinding.SessionState{Clock: clock, ProjectionBackendScope: "selected-backend"}
	selected.ModelInvocation.RuntimeID = "prior-runtime"
	execution := &bindingExecution{bind: func(string, string, string, providers.Service, platformprocess.CommandRunner,
		workers.ProgressPublisher, func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)) (func(), error) {
		return nil, cause
	}}
	cleanup := &runtimeOpeningCleanup{}
	err := NewRuntimeOpeningBinding(nil, nil, nil, nil).Bind(t.Context(), RuntimeOpeningBindingRequest{
		Facts: roles.SessionOpeningFacts{FactorySessionID: "selected", RuntimeID: "candidate-runtime"},
	}, selected, clock, inertHostedInstance{}, bindingSessionRuntime{}, nil,
		&factoryruntime.RuntimeActivation{}, execution, nil, cleanup)
	var typed *factorysessions.DetachedRequestError
	if !errors.Is(err, cause) || !errors.As(err, &typed) || selected.ModelInvocation.RuntimeID != "prior-runtime" ||
		selected.Clock != clock || selected.ProjectionBackendScope != "selected-backend" {
		t.Fatalf("failed binding changed selected facts: %+v, %v", selected, err)
	}
}

func assertBindingTypedFailure(t *testing.T, opened *runtimebinding.SessionState, err, cause error, lookups int) {
	t.Helper()
	var typed *factorysessions.DetachedRequestError
	if !errors.Is(err, cause) || !errors.As(err, &typed) || opened.Clock != nil || opened.ModelInvocation.RuntimeID != "" || lookups != 0 {
		t.Fatalf("final-bind failure = %+v, %v; lookups=%d", opened, err, lookups)
	}
}

func assertBindingChildEffects(t *testing.T, id, key, generation string, provider providers.Service, runner platformprocess.CommandRunner,
	gotID, runtimeID, gotGeneration string, gotProvider providers.Service, gotRunner platformprocess.CommandRunner,
	publish workers.ProgressPublisher,
	starter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) {
	t.Helper()

	if gotID != id || runtimeID != "runtime-"+id || gotGeneration != generation || gotProvider != provider || gotRunner != runner {
		t.Fatalf("selected child binding changed: %s %s %s", gotID, runtimeID, gotGeneration)
	}
	result, err := gotRunner.Run(t.Context(), platformprocess.CommandRequest{WorkDir: key})
	if err != nil {
		t.Fatal(err)
	}
	publish(workers.ProgressFragment{Payload: string(result.Stdout)})
	finish, err := starter(t.Context(), &workers.ExecuteRequest{Correlation: workers.ExecutionCorrelation{FactorySessionID: id}})
	if err != nil || finish == nil {
		t.Fatal("lost selected attempt handle")
	}
}

func closeBindingFixture(t *testing.T, cleanup *runtimeOpeningCleanup) {
	t.Helper()
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeOpeningBindingUnavailableCapabilitiesDeferPublication(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"runtime", "execution"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			startup := &bindingRecordingStartup{}
			var session roles.ApplicationRuntime = bindingSessionRuntime{}
			if missing == "runtime" {
				session = &completionSession{}
			}
			cleanup := &runtimeOpeningCleanup{}
			opened := &runtimebinding.SessionState{}
			err := NewRuntimeOpeningBinding(nil, nil, nil, nil).Bind(t.Context(), RuntimeOpeningBindingRequest{},
				opened, openingCoordinatorClock{}, startup, session, nil, &factoryruntime.RuntimeActivation{},
				nil, nil, cleanup)
			if err == nil || !startup.deferred || opened.Clock != nil || opened.ModelInvocation.RuntimeID != "" || len(cleanup.actions) != 0 {
				t.Fatalf("unavailable %s = %+v, %v, deferred=%v", missing, opened, err, startup.deferred)
			}
		})
	}
}

type bindingRecordingStartup struct {
	inertHostedInstance
	deferred bool
}

func (startup *bindingRecordingStartup) DeferRecordingPublication() { startup.deferred = true }
func (*bindingRecordingStartup) ActivateRecordingPublication(context.Context) error {
	panic("final binding published recording")
}

type bindingIdentityAdapter struct{ gateway roles.SessionGateway }

func (reader bindingIdentityAdapter) ResolveOpeningSessionIdentity(ctx context.Context, id string) (string, error) {
	projection, err := reader.gateway.GetFactorySession(ctx, id)
	return projection.Context.FactorySessionID, err
}
func bindingIdentityReader(gateway roles.SessionGateway) OpeningSessionIdentity {
	if gateway == nil {
		return nil
	}
	return bindingIdentityAdapter{gateway: gateway}
}
