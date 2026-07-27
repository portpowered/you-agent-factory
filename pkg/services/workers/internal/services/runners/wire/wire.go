// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
)

// NewService validates registrations into one immutable private registry.
func NewService(registrations []runners.Registration) (runners.Service, error) {
	return internalservice.New(registrations)
}

// NewScriptRegistry constructs one Script Runner from explicit effects and
// publishes it through the immutable private registry.
func NewScriptRegistry(
	config runners.ScriptConfig,
	dependencies runners.ScriptDependencies,
) (runners.Service, error) {
	implementation, err := script.New(
		script.Config{
			Command:          config.Command,
			Args:             append([]string(nil), config.Args...),
			FactoryDirectory: config.FactoryDirectory,
		},
		script.Dependencies{
			CommandRunner: dependencies.CommandRunner,
			FactoryDocs:   dependencies.FactoryDocs,
			Now:           dependencies.Now,
			Publish:       dependencies.Publish,
			Record:        dependencies.Record,
		},
	)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.ScriptIdentity,
		Metadata: scriptMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
}

// NewInferenceRegistry constructs one Inference Runner from explicit effects
// and publishes it through the immutable private registry.
func NewInferenceRegistry(
	config runners.InferenceConfig,
	dependencies runners.InferenceDependencies,
) (runners.Service, error) {
	implementation, err := inference.New(
		inference.Config{
			Worker: snapshotInferenceWorker(config.Worker),
			Resources: append(
				[]models.LocalResource(nil),
				config.Resources...,
			),
		},
		inference.Dependencies{
			Models:   dependencies.Models,
			Delegate: dependencies.Delegate,
		},
	)
	service, registryErr := NewService([]runners.Registration{{
		Identity: runners.InferenceIdentity,
		Metadata: inferenceMetadata(),
		Runner:   implementation,
	}})
	return service, errors.Join(err, registryErr)
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
