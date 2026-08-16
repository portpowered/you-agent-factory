package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

func TestRuntimeRunnerPreflightUsesProvidersRoot(t *testing.T) {
	provider := &runtimePreflightProvider{}

	selection, err := resolveRuntimeRunnerSelection(
		t.Context(),
		provider,
		"workstation-provider",
		"factory-provider",
		"model-provider",
	)
	if err != nil {
		t.Fatalf("resolveRuntimeRunnerSelection() error = %v", err)
	}
	if selection.RunnerID != "antigravity" || selection.Source != workers.RunnerSelectionSourceWorkstation {
		t.Fatalf("selection = %#v, want Providers-root selection", selection)
	}
	if provider.selection != (providers.ResolveSelectionRequest{
		Workstation:   "workstation-provider",
		Factory:       "factory-provider",
		ModelProvider: "model-provider",
	}) {
		t.Fatalf("selection request = %#v", provider.selection)
	}

	if err := validateRuntimeRunnerIdentity(t.Context(), provider, "provider-alias"); err != nil {
		t.Fatalf("validateRuntimeRunnerIdentity() error = %v", err)
	}
	if err := validateRuntimeRunnerPrerequisites(t.Context(), provider, nil, "provider-alias"); err != nil {
		t.Fatalf("validateRuntimeRunnerPrerequisites() error = %v", err)
	}
	if len(provider.identities) != 2 || provider.identities[0] != "provider-alias" || provider.identities[1] != "provider-alias" {
		t.Fatalf("identity requests = %#v", provider.identities)
	}
	if provider.prerequisite != providers.IDAntigravity {
		t.Fatalf("prerequisite request = %q, want antigravity", provider.prerequisite)
	}
}

type runtimePreflightProvider struct {
	testutil.NativeProvider
	selection    providers.ResolveSelectionRequest
	identities   []string
	prerequisite providers.ID
}

func (provider *runtimePreflightProvider) ResolveSelection(
	_ context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	provider.selection = request
	return providers.ResolveSelectionResult{
		Provider: providers.IDAntigravity,
		Source:   providers.SelectionSourceWorkstation,
	}, nil
}

func (provider *runtimePreflightProvider) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	provider.identities = append(provider.identities, request.Identity)
	return providers.ResolveIdentityResult{ID: providers.IDAntigravity}, nil
}

func (provider *runtimePreflightProvider) ValidatePrerequisites(
	_ context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	provider.prerequisite = request.ID
	return nil
}

// TestBuildRuntimeExecutorsOmitsAutomationsOwnedPollerWorkers proves hosted and
// poller Worker shapes never enter Workers executor construction, including the
// former NoopExecutor silent-accept fallback.
func TestBuildRuntimeExecutorsOmitsAutomationsOwnedPollerWorkers(t *testing.T) {
	t.Parallel()

	for _, workerType := range []string{
		interfaces.WorkerTypeHosted,
		interfaces.WorkerTypePoller,
	} {
		workerType := workerType
		t.Run(workerType, func(t *testing.T) {
			t.Parallel()

			worker := &interfaces.FactoryWorkerConfig{
				Name:     "linear-poller",
				Type:     workerType,
				Provider: interfaces.HostedWorkerProviderLinear,
			}
			factoryConfig := &interfaces.FactoryConfig{
				Workers: []interfaces.FactoryWorkerConfig{*worker},
				Workstations: []interfaces.FactoryWorkstationConfig{{
					Name:           "poll-linear",
					Kind:           interfaces.WorkstationKindPoller,
					WorkerTypeName: worker.Name,
				}},
			}
			runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
				Factory: factoryConfig,
				Workers: map[string]*interfaces.FactoryWorkerConfig{
					worker.Name: worker,
				},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{
					"poll-linear": {
						Name:           "poll-linear",
						Kind:           interfaces.WorkstationKindPoller,
						WorkerTypeName: worker.Name,
					},
				},
			}
			builder := &pollerOmissionBuilder{t: t}
			service := &Service{
				executorBuilder:   builder,
				clock:             time.Now,
				executableLocator: stubExecutableLocator{},
			}

			executors, err := service.BuildRuntimeExecutors(
				runtimeConfig,
				factoryConfig,
				"",
				nil,
				logging.NoopLogger{},
				true,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				time.Now,
			)
			if err != nil {
				t.Fatalf("BuildRuntimeExecutors() error = %v", err)
			}
			if builder.calls != 0 {
				t.Fatalf("executor builder calls = %d, want zero for Automations-owned pollers", builder.calls)
			}
			if _, ok := executors[worker.Name]; ok {
				t.Fatalf("executors[%q] present, want omitted (not NoopExecutor)", worker.Name)
			}
			if len(executors) != 0 {
				t.Fatalf("executors = %#v, want empty map", executors)
			}
		})
	}
}

func TestBuildRuntimeExecutorsLeavesScriptWorkersToDetachedService(t *testing.T) {
	t.Parallel()

	worker := &interfaces.FactoryWorkerConfig{
		Name:    "script-worker",
		Type:    interfaces.WorkerTypeScript,
		Command: "script-tool",
	}
	factoryConfig := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{*worker},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory: factoryConfig,
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			worker.Name: worker,
		},
	}
	builder := &pollerOmissionBuilder{t: t}
	service := &Service{
		executorBuilder:   builder,
		clock:             time.Now,
		executableLocator: stubExecutableLocator{},
	}

	executors, err := service.BuildRuntimeExecutors(
		runtimeConfig,
		factoryConfig,
		"",
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
	)
	if err != nil {
		t.Fatalf("BuildRuntimeExecutors() error = %v", err)
	}
	if builder.calls != 0 {
		t.Fatalf("executor builder calls = %d, want zero for detached script execution", builder.calls)
	}
	if _, ok := executors[worker.Name]; ok {
		t.Fatalf("executors[%q] present, want script execution owned by detached service", worker.Name)
	}
}

func TestBuildRuntimeExecutorsLeavesInferenceWorkersToDetachedService(t *testing.T) {
	t.Parallel()

	worker := &interfaces.FactoryWorkerConfig{
		Name: "inference-worker", Type: interfaces.WorkerTypeInference,
		Model: "local-model", ModelLocality: interfaces.ModelLocalityLocal,
	}
	factoryConfig := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{*worker},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory: factoryConfig,
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			worker.Name: worker,
		},
	}
	builder := &pollerOmissionBuilder{t: t}
	service := &Service{
		executorBuilder:   builder,
		clock:             time.Now,
		executableLocator: stubExecutableLocator{},
	}

	executors, err := service.BuildRuntimeExecutors(
		runtimeConfig,
		factoryConfig,
		"",
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
	)
	if err != nil {
		t.Fatalf("BuildRuntimeExecutors() error = %v", err)
	}
	if builder.calls != 0 {
		t.Fatalf("executor builder calls = %d, want zero for detached inference execution", builder.calls)
	}
	if _, ok := executors[worker.Name]; ok {
		t.Fatalf("executors[%q] present, want inference execution owned by detached service", worker.Name)
	}
}

type pollerOmissionBuilder struct {
	t     *testing.T
	calls int
}

func (b *pollerOmissionBuilder) Build(
	interfaces.RuntimeConfigLookup,
	string,
	string,
	*workers.Context,
	logging.Logger,
	*bool,
	providers.Service,
	workers.ProgressPublisher,
	workers.ScriptEventRecorder,
	workers.InferenceEventRecorder,
	workeragentrun.AgentRunEventRecorder,
	func() time.Time,
	func() []string,
	func() (string, error),
	[]workerconstruction.RunnerDecorator,
) (workerconstruction.Result, error) {
	b.calls++
	b.t.Fatal("executor builder must not construct Automations-owned poller workers")
	return workerconstruction.Result{}, fmt.Errorf("unexpected poller executor construction")
}

func (b *pollerOmissionBuilder) BuildLogical(
	interfaces.RuntimeConfigLookup,
	string,
	string,
	*workers.Context,
	logging.Logger,
	func() time.Time,
	func() []string,
	func() (string, error),
) workerconstruction.Result {
	b.calls++
	b.t.Fatal("logical builder must not construct Automations-owned poller workers")
	return workerconstruction.Result{}
}

type stubExecutableLocator struct{}

func (stubExecutableLocator) LookPath(string) (string, error) {
	return "", fmt.Errorf("executable lookup disabled in poller omission test")
}
