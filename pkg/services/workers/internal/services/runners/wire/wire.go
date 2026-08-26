// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/mock"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
	agentwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/wire"
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
	return newProductionRegistry(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		nil,
		runners.MockDependencies{},
	)
}

// NewMockProductionRegistry constructs the explicit Workers mock-feature
// graph. It uses the same immutable registry as production composition, with
// one additional request-selected mock registration. Production composition
// must continue to call NewProductionRegistry so mock is absent there.
func NewMockProductionRegistry(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
	mockConfig runners.MockConfig,
	mockDependencies runners.MockDependencies,
) (runners.Service, error) {
	return newProductionRegistry(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		&mockConfig,
		mockDependencies,
	)
}

func newProductionRegistry(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
	mockConfig *runners.MockConfig,
	mockDependencies runners.MockDependencies,
) (runners.Service, error) {
	agentImplementation, err := agentImplementation(agentDependencies)
	if err != nil {
		return nil, invalidRunnerConstruction(runners.AgentIdentity, err)
	}
	scriptImplementation, err := scriptImplementation(scriptConfig, scriptDependencies)
	if err != nil {
		return nil, invalidRunnerConstruction(runners.ScriptIdentity, err)
	}
	if inferenceDependencies.Delegate == nil {
		// Managed inference first asks Models and then uses the canonical Agent
		// Runner for provider fallback. Both strategies remain private registry
		// entries behind the same Workers.Execute call.
		inferenceDependencies.Delegate = agentImplementation
	}
	inferenceImplementation, err := inferenceImplementation(
		inferenceConfig,
		inferenceDependencies,
	)
	if err != nil {
		return nil, invalidRunnerConstruction(runners.InferenceIdentity, err)
	}
	registrations := []runners.Registration{
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
	}
	if mockConfig != nil {
		mockImplementation, mockErr := mock.New(
			mock.Config{WorkersConfig: mockConfig.WorkersConfig},
			mock.Dependencies{Next: mockDependencies.Next, Files: mockDependencies.Files},
		)
		if mockErr != nil {
			return nil, invalidRunnerConstruction(runners.MockIdentity, mockErr)
		}
		registrations = append(registrations, runners.Registration{
			Identity: runners.MockIdentity,
			Metadata: mockMetadata(),
			Runner:   mockImplementation,
		})
	}
	return NewService(registrations)
}

func invalidRunnerConstruction(identity string, err error) error {
	return fmt.Errorf(
		"%w: %s runner construction failed: %w",
		workers.ErrInvalidRunnerRegistration,
		identity,
		err,
	)
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

func snapshotInferenceWorker(worker models.LocalWorker) models.LocalWorker {
	worker.Resources = append([]models.LocalResource(nil), worker.Resources...)
	return worker
}

func scriptMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          runners.ScriptIdentity,
		DisplayName: "Script",
		Capabilities: workers.NewCapabilities(
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
		Capabilities: workers.NewCapabilities(
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
		Capabilities: workers.NewCapabilities(
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
		Capabilities: workers.NewCapabilities(
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
