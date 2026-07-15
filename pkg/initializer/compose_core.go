package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/composebridge"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

// ComposeRoot holds the absolutized factory directory and base logger after
// config normalization during initializer composition.
type ComposeRoot = composebridge.Root

// ComposeCollaborators groups explicit composition collaborators.
type ComposeCollaborators = composebridge.Collaborators

// ComposeConfigLoad carries factory config load outputs needed before runtime
// bundle construction.
type ComposeConfigLoad = composebridge.ConfigLoad

// buildCore constructs the normalized runtime graph without attaching a transport host.
func buildCore(ctx context.Context, cfg *Config) (*Core, error) {
	return composebridge.BuildCore(ctx, cfg)
}

// ModelService is the transport-facing model catalog seam.
type ModelService = composebridge.ModelService

// FactoryDefinitionService is the transport-facing factory definition seam.
type FactoryDefinitionService = composebridge.FactoryDefinitionService

// LocalModelDomain groups local-model collaborators composed during startup.
type LocalModelDomain = composebridge.LocalModelDomain

// WorkersSchedulerService is the worker scheduling collaborator composed during startup.
type WorkersSchedulerService = workersservice.Service

// ReplayArtifact is the replay metadata loaded during compose config loading.
type ReplayArtifact = interfaces.ReplayArtifact

// LoadedFactoryConfig is the loaded factory.json runtime config.
type LoadedFactoryConfig = factoryconfig.LoadedFactoryConfig

// AssetPuller pulls managed local model assets during startup composition.
type AssetPuller = localmodels.AssetPuller
