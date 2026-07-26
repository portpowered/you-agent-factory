// Package wire constructs the private Workers Runners subservice.
package wire

import (
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
	config script.Config,
	dependencies script.Dependencies,
) (runners.Service, error) {
	implementation, err := script.New(config, dependencies)
	if err != nil {
		return nil, err
	}
	return NewService([]runners.Registration{{
		Identity: script.Identity,
		Metadata: scriptMetadata(),
		Runner:   implementation,
	}})
}

func scriptMetadata() workers.RunnerMetadata {
	return workers.RunnerMetadata{
		ID:          script.Identity,
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
