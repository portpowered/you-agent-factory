// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/service"
)

var executeCLI = cli.Execute

func main() {
	run.SetBuildFactoryService(func(ctx context.Context, cfg *service.FactoryServiceConfig) (run.RuntimeRunner, error) {
		return compose.InjectCLIRunner(ctx, cfg)
	})
	executeCLI()
}
