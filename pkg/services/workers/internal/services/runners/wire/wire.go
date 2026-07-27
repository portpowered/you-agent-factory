// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
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
