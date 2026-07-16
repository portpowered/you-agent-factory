//go:build wireinject

package wire

import (
	"context"
	"fmt"

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
	return buildRuntimeCore(ctx, cfg)
}

// InjectFactoryService is the wireinject entry for the factory composition root.
func InjectFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	runtimeCfg := service.RuntimeHostConfigFromFactoryService(cfg)
	if runtimeCfg == nil {
		return nil, fmt.Errorf("factory service config is required")
	}
	copied := *runtimeCfg
	core, err := buildRuntimeCore(ctx, &copied)
	if err != nil {
		return nil, err
	}
	shell := service.FactoryServiceShell{Service: service.NewFactoryServiceFromRuntimeHostCore(core)}
	svc := service.AttachModelServiceCollaborator(shell, core.ModelService())
	return service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: svc},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: svc}, cfg),
	), nil
}
