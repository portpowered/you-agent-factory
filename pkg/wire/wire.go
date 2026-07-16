//go:build wireinject

package wire

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var WorkerApplicationSet = wire.NewSet(
	provideProviderCommandEdge,
	provideScriptCommandEdge,
	provideProviderPTYEdge,
	provideWorkerProviderFactory,
	provideWorkerScriptFactory,
	provideWorkerHostedConfig,
	provideWorkerApplication,
)

// InjectWireCore is the wireinject entry for the process composition surface.
func InjectWireCore() WireCore {
	wire.Build(
		provideCLICommandBuilder,
		provideProcessGraphBuilder,
		provideProcessInitializer,
		provideMCPExecutionBuilder,
		provideSessionExecutionBuilder,
		provideModelInvocationBuilder,
		provideWorkerApplicationBuilder,
		provideRunSessionExecutionBuilder,
		wire.Struct(new(WireCore), "*"),
	)
	return WireCore{}
}

// InjectWorkerApplication is the wireinject entry for process-selected worker
// factories and their production or functional side-effect edges.
func InjectWorkerApplication(logger *zap.Logger, edges FunctionalEdges) (workerapplication.Components, error) {
	wire.Build(WorkerApplicationSet)
	return workerapplication.Components{}, nil
}

// InjectCLICommand is the wireinject entry for the Cobra CLI command tree.
func InjectCLICommand(options cli.RootCommandOptions) *cobra.Command {
	wire.Build(cli.NewRootCommandWithOptions)
	return nil
}

// InjectRuntimeCore constructs the single Factory Session core consumed by the
// application graph before initializer lifecycle execution.
func InjectRuntimeCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	wire.Build(
		provideRuntimeHostRoot,
		provideRuntimeHostBaseLogger,
		provideFactorySessionsRegistry,
		provideRuntimeHostConfigLoad,
		provideRuntimeHostClock,
		provideRuntimeHostLocalModels,
		provideRuntimeHostRuntimeBuild,
		provideRuntimeHostPersistence,
		provideRuntimeHostDurableExecution,
		provideRuntimeHostRecordingBuild,
		provideRuntimeHostHostedWorkers,
		provideRuntimeHostWorkers,
		provideRuntimeHostCollaborators,
		provideRuntimeHostCore,
		provideRuntimeModelServiceDependencies,
		provideRuntimeModelService,
		provideRuntimeHostCoreWithModels,
	)
	return nil, nil
}

// InjectFactoryService is the wireinject entry for the factory composition root.
func InjectFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	wire.Build(
		provideRuntimeHostConfigFromFactoryService,
		provideRuntimeHostRoot,
		provideRuntimeHostBaseLogger,
		provideFactorySessionsRegistry,
		provideRuntimeHostConfigLoad,
		provideRuntimeHostClock,
		provideRuntimeHostLocalModels,
		provideRuntimeHostRuntimeBuild,
		provideRuntimeHostPersistence,
		provideRuntimeHostDurableExecution,
		provideRuntimeHostRecordingBuild,
		provideRuntimeHostHostedWorkers,
		provideRuntimeHostWorkers,
		provideRuntimeHostCollaborators,
		provideRuntimeHostCore,
		provideRuntimeModelServiceDependencies,
		provideRuntimeModelService,
		provideRuntimeHostCoreWithModels,
		provideFactoryServiceFromRuntimeHostCore,
	)
	return nil, nil
}
