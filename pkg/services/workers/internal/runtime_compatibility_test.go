package internal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

func TestRootCompatibilityDelegatesAdmissionAndReplacement(t *testing.T) {
	t.Parallel()

	firstAssembly := &recordingRuntimeAssembly{}
	secondAssembly := &recordingRuntimeAssembly{
		result: workers.RuntimeBuildResult{RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
		}},
	}
	replaced := RootFrom(firstAssembly, workstationswire.NewService())
	replaced = replaced.ReplaceRuntimeAssembly(secondAssembly)
	replaced = replaced.ReplaceWorkstations(workstationswire.NewService())

	if _, err := replaced.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{}); err != nil {
		t.Fatalf("BuildRuntime() after runtime replacement error = %v", err)
	}
	if secondAssembly.request.RunnerID != "" {
		t.Fatalf("replacement runtime received unexpected runner ID %q", secondAssembly.request.RunnerID)
	}

	service := workers.Service(&replaced)
	if _, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: workers.RunnerIDCodex,
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			Executor: &compatibilityWorkstationExecutor{},
		}},
	}); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	var admitted bool
	result, err := service.DispatchWorkstationWithAdmission(
		context.Background(),
		rootDispatchRequest("compatibility-dispatch", workers.RunnerIDCodex),
		func() { admitted = true },
	)
	if err != nil {
		t.Fatalf("DispatchWorkstationWithAdmission() error = %v", err)
	}
	if !admitted || result.DispatchID != "compatibility-dispatch" {
		t.Fatalf("admission/result = %t/%#v, want admitted dispatch", admitted, result)
	}

	if _, err := service.StopWorkstationPool(context.Background()); err != nil {
		t.Fatalf("StopWorkstationPool() error = %v", err)
	}
	if _, err := service.InvokeModel(context.Background(), "model", models.Request{}); err == nil {
		t.Fatal("Root.InvokeModel() error = nil, want unavailable runtime")
	}

	delegating := startedRootService(t, "service-admission", &compatibilityWorkstationExecutor{})
	var serviceAdmission bool
	if _, err := delegating.DispatchWorkstationWithAdmission(
		context.Background(),
		rootDispatchRequest("service-admission-dispatch", "service-admission"),
		func() { serviceAdmission = true },
	); err != nil || !serviceAdmission {
		t.Fatalf("Service.DispatchWorkstationWithAdmission() = %v, admitted=%t; want successful admission", err, serviceAdmission)
	}
	if err := (*Service)(nil).Close(context.Background()); err != nil {
		t.Fatalf("nil Service.Close() error = %v, want nil", err)
	}
}

func TestNilRootCapabilitiesRemainUnavailable(t *testing.T) {
	t.Parallel()

	var root *Root
	if _, err := root.Execute(context.Background(), workers.ExecuteRequest{}); !errors.Is(err, workers.ErrExecuteUnavailable) {
		t.Fatalf("nil Root.Execute() error = %v, want ErrExecuteUnavailable", err)
	}
	if _, err := root.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{}); !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
		t.Fatalf("nil Root.BuildRuntime() error = %v, want ErrIncompleteRuntimeAssembly", err)
	}
	if _, err := root.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.StartWorkstationPool() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.StopWorkstationPool(context.Background()); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.StopWorkstationPool() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.WorkstationRoute() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.DispatchWorkstation(context.Background(), workers.WorkstationDispatchRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.DispatchWorkstation() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.DispatchWorkstationWithAdmission(context.Background(), workers.WorkstationDispatchRequest{}, nil); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.DispatchWorkstationWithAdmission() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.CancelWorkstationDispatch(context.Background(), workers.WorkstationDispatchCancelRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.CancelWorkstationDispatch() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
}

func TestOwnedProviderLifecyclesClosesInReverseAndRejectsLateServices(t *testing.T) {
	t.Parallel()

	var closed []string
	owned := &ownedProviderLifecycles{}
	if !owned.Add(testProvidersService{}) {
		t.Fatal("Add(non-lifecycle service) = false, want true")
	}
	if !owned.Add(&compatibilityProviderLifecycle{label: "first", closed: &closed, err: errors.New("first close")}) {
		t.Fatal("Add(first lifecycle) = false, want true")
	}
	if !owned.Add(&compatibilityProviderLifecycle{label: "second", closed: &closed}) {
		t.Fatal("Add(second lifecycle) = false, want true")
	}

	err := owned.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "first close") {
		t.Fatalf("Close() error = %v, want joined lifecycle error", err)
	}
	if strings.Join(closed, ",") != "second,first" {
		t.Fatalf("closed lifecycle order = %v, want reverse registration order", closed)
	}
	if owned.Add(&compatibilityProviderLifecycle{label: "late", closed: &closed}) {
		t.Fatal("Add(after Close) = true, want false")
	}
	if err := owned.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}

	var nilOwned *ownedProviderLifecycles
	if nilOwned.Add(testProvidersService{}) {
		t.Fatal("nil owned lifecycle Add() = true, want false")
	}
	if err := nilOwned.Close(context.Background()); err != nil {
		t.Fatalf("nil owned lifecycle Close() error = %v, want nil", err)
	}
}

func TestRuntimeCompatibilityRejectsUnsupportedAndUsesInjectedExecutor(t *testing.T) {
	t.Parallel()

	if _, err := BuildRuntimeExecutors(nil, nil, nil, "", nil, nil, false, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("BuildRuntimeExecutors(nil) error = nil, want unsupported runtime error")
	}
	if _, err := (runtimeAssemblyRunner{}).Execute(context.Background(), workers.RunnerExecutionRequest{}); !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
		t.Fatalf("runtime assembly runner error = %v, want ErrIncompleteRuntimeAssembly", err)
	}

	service := &Service{}
	if service.CurrentModelRuntimeConfig() != nil {
		t.Fatal("CurrentModelRuntimeConfig() != nil, want no implicit runtime")
	}
	var called bool
	want := &compatibilityWorkstationExecutor{}
	service.modelInvocationExecutorOverride = func(
		interfaces.RuntimeConfigLookup,
		*interfaces.FactoryConfig,
		string,
	) (workers.WorkstationRequestExecutor, error) {
		called = true
		return want, nil
	}
	got, err := service.modelInvocationExecutor(nil, nil, "worker")
	if err != nil || got != want || !called {
		t.Fatalf("modelInvocationExecutor() = %#v, %v, called=%t; want injected executor", got, err, called)
	}
}

func TestRuntimeAssemblyRegistrationsUseProviderRegistryMetadata(t *testing.T) {
	t.Parallel()

	registrations, err := runtimeAssemblyRegistrations(compatibilityProviderRegistry{})
	if err != nil {
		t.Fatalf("runtimeAssemblyRegistrations() error = %v", err)
	}
	if len(registrations) != 1 || registrations[0].Identity != workers.RunnerIDCodex {
		t.Fatalf("runtime assembly registrations = %#v, want one Codex registration", registrations)
	}
}

func TestServiceCommandClockUsesInjectedClockAndSafeFallback(t *testing.T) {
	t.Parallel()

	fallback := serviceCommandClock(nil).Now()
	if fallback.IsZero() {
		t.Fatal("serviceCommandClock(nil) returned zero time")
	}
	want := time.Unix(42, 0)
	if got := serviceCommandClock(&Service{clock: func() time.Time { return want }}).Now(); !got.Equal(want) {
		t.Fatalf("serviceCommandClock(injected) = %v, want %v", got, want)
	}
}

type compatibilityProviderLifecycle struct {
	providers.Service
	label  string
	closed *[]string
	err    error
}

func (lifecycle *compatibilityProviderLifecycle) Close(context.Context) error {
	*lifecycle.closed = append(*lifecycle.closed, lifecycle.label)
	return lifecycle.err
}

type compatibilityWorkstationExecutor struct{}

func (compatibilityWorkstationExecutor) Execute(
	context.Context,
	workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

type compatibilityProviderRegistry struct{}

func (compatibilityProviderRegistry) UsesNativeRunner(string) bool { return true }

func (compatibilityProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}

func (compatibilityProviderRegistry) RunnerIdentities() []string {
	return []string{workers.RunnerIDCodex}
}

func (compatibilityProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	metadata, _ := workerrunner.BuiltInRunnerMetadata(workers.RunnerIDCodex)
	return metadata, nil
}

func (compatibilityProviderRegistry) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
}

func (compatibilityProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{RunnerID: workers.RunnerIDCodex}, nil
}
