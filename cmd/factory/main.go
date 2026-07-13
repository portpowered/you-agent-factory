// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"
	"os"

	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

var executeCLI = cli.Execute

func main() {
	run.SetBuildFactoryService(buildCLIRunner)
	executeCLI()
}

func buildCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (run.RuntimeRunner, error) {
	runtimeCfg := service.RuntimeHostConfigFromFactoryService(cfg)
	if runtimeCfg != nil {
		copied := *runtimeCfg
		runtimeCfg = &copied
		if runtimeCfg.Logger == nil {
			runtimeCfg.Logger = zap.NewNop()
		}
		runtimeCfg.Clock = factory.EnsureClock(runtimeCfg.Clock)
	}
	application, err := root.Start(ctx, root.Inputs{
		Mode: initializer.ModeCLI,
		Graph: wire.Inputs{
			Config:    runtimeCfg,
			MCPInput:  os.Stdin,
			MCPOutput: os.Stdout,
		},
	})
	if err != nil {
		return nil, err
	}
	return application, nil
}
