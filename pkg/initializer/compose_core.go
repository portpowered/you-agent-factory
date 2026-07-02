package initializer

import (
	"context"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/composebridge"
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

// ComposeCore constructs a runtimehost.Core using explicit composition collaborators.
func ComposeCore(
	ctx context.Context,
	cfg *Config,
	root ComposeRoot,
	collaborators ComposeCollaborators,
	load ComposeConfigLoad,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*Core, error) {
	return composebridge.ComposeCore(ctx, cfg, root, collaborators, load, clock, hostedWorkers)
}

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
