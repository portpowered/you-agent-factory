// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"errors"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/mock"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
	agentwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/wire"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
)

// NewService validates registrations into one immutable private registry.
func NewService(registrations []runners.Registration) (runners.Service, error) {
	return internalservice.New(registrations)
}

// NewProductionRegistry constructs the three production strategies and
// publishes them together through one immutable registry. Construction only
// snapshots configuration and validates collaborators; it never resolves or
// executes a strategy. The mock strategy is intentionally absent and is
// composed only by the explicit Workers mock feature path.
func NewProductionRegistry(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
) (runners.Service, error) {
	agentImplementation, err := agentImplementation(agentDependencies)
	if err != nil {
		return nil, err
	}
	scriptImplementation, err := scriptImplementation(scriptConfig, scriptDependencies)
	if err != nil {
		return nil, err
	}
	inferenceImplementation, err := inferenceImplementation(
		inferenceConfig,
		inferenceDependencies,
	)
	if err != nil {
		return nil, err
	}
	return NewService([]runners.Registration{
		{
			Identity: runners.AgentIdentity,
			Metadata: agentMetadata(),
			Runner:   agentImplementation,
		},
		{
			Identity: runners.ScriptIdentity,
			Metadata: scriptMetadata(),
			Runner:   scriptImplementation,
		},
		{
			Identity: runners.InferenceIdentity,
			Metadata: inferenceMetadata(),
			Runner:   inferenceImplementation,
		},
	})
}

// NewScriptRegistry constructs one Script Runner from explicit effects and
// publishes it through the immutable private registry.
func NewScriptRegistry(
	config runners.ScriptConfig,
	dependencies runners.ScriptDependencies,
) (runners.Service, error) {
	implementation, err := scriptImplementation(config, dependencies)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.ScriptIdentity,
		Metadata: scriptMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
}

func scriptImplementation(
	config runners.ScriptConfig,
	dependencies runners.ScriptDependencies,
) (workers.Runner, error) {
	return script.New(
		script.Config{
			Command:          config.Command,
			Args:             append([]string(nil), config.Args...),
			FactoryDirectory: config.FactoryDirectory,
			RequestSelected:  config.RequestSelected,
		},
		script.Dependencies{
			CommandRunner: dependencies.CommandRunner,
			FactoryDocs:   dependencies.FactoryDocs,
			Now:           dependencies.Now,
			Publish:       dependencies.Publish,
			Record:        dependencies.Record,
		},
	)
}

// NewInferenceRegistry constructs one Inference Runner from explicit effects
// and publishes it through the immutable private registry.
func NewInferenceRegistry(
	config runners.InferenceConfig,
	dependencies runners.InferenceDependencies,
) (runners.Service, error) {
	implementation, err := inferenceImplementation(config, dependencies)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.InferenceIdentity,
		Metadata: inferenceMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
}

func inferenceImplementation(
	config runners.InferenceConfig,
	dependencies runners.InferenceDependencies,
) (workers.Runner, error) {
	return inference.New(
		inference.Config{
			Worker: snapshotInferenceWorker(config.Worker),
			Resources: append(
				[]models.LocalResource(nil),
				config.Resources...,
			),
			Scope: config.Scope,
		},
		inference.Dependencies{
			Models:   dependencies.Models,
			Delegate: dependencies.Delegate,
		},
	)
}

// NewAgentRegistry constructs one inert Agent Runner over the singular
// Providers root and publishes it through the immutable private registry.
func NewAgentRegistry(
	dependencies runners.AgentDependencies,
) (runners.Service, error) {
	implementation, err := agentImplementation(dependencies)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.AgentIdentity,
		Metadata: agentMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
}

func agentImplementation(
	dependencies runners.AgentDependencies,
) (workers.Runner, error) {
	return agentwire.NewService(
		dependencies.Providers,
		dependencies.Publish,
		dependencies.DecisionEnvelopes,
	)
}

// NewMockRegistry constructs one mock Strategy for the Workers-owned testing
// feature path. It must not be composed into production Workers wire.
func NewMockRegistry(
	config runners.MockConfig,
	dependencies runners.MockDependencies,
) (runners.Service, error) {
	implementation, err := mock.New(
		mock.Config{WorkersConfig: config.WorkersConfig},
		mock.Dependencies{Next: dependencies.Next},
	)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.MockIdentity,
		Metadata: mockMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
}

// NewInferenceCompositionRunner resolves one registry-backed Inference Runner
// that projects managed-runtime invocation ahead of the supplied delegate.
func NewInferenceCompositionRunner(
	inner workers.Runner,
	modelsService inference.LocalInvoker,
	modelsScope models.RuntimeScopeRef,
	worker *interfaces.FactoryWorkerConfig,
	resources []interfaces.ResourceConfig,
) workers.Runner {
	if inner == nil || modelsService == nil || worker == nil {
		return inner
	}
	registry, err := NewInferenceRegistry(
		runners.InferenceConfig{
			Worker:    inference.WorkerFromFactory(worker),
			Resources: inference.ResourcesFromFactory(resources),
			Scope:     modelsScope,
		},
		runners.InferenceDependencies{
			Models:   modelsService,
			Delegate: inner,
		},
	)
	if err != nil {
		return inner
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: runners.InferenceIdentity,
	})
	if err != nil {
		return inner
	}
	return binding.Runner
}

func snapshotInferenceWorker(worker models.LocalWorker) models.LocalWorker {
	worker.Resources = append([]models.LocalResource(nil), worker.Resources...)
	return worker
}

func scriptMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          runners.ScriptIdentity,
		DisplayName: "Script",
		Capabilities: workerrunner.NewCapabilities(
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilitySessionResume,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityStructuredOutput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorkingDirectory,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorktree,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
		),
	}
}

func inferenceMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          runners.InferenceIdentity,
		DisplayName: "Inference",
		Capabilities: workerrunner.NewCapabilities(
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilitySessionResume,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityStructuredOutput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorkingDirectory,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorktree,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
		),
	}
}

func agentMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          runners.AgentIdentity,
		DisplayName: "Agent",
		Capabilities: workerrunner.NewCapabilities(
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilitySessionResume,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityStructuredOutput,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorkingDirectory,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorktree,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
		),
	}
}

func mockMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          runners.MockIdentity,
		DisplayName: "Mock",
		Capabilities: workerrunner.NewCapabilities(
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilitySessionResume,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityStructuredOutput,
				Status:     workers.RunnerOptionalCapabilityStatusUnsupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorkingDirectory,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
			workers.RunnerOptionalCapabilitySupport{
				Capability: workers.RunnerOptionalCapabilityWorktree,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			},
		),
	}
}
