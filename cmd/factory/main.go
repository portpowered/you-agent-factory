// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/service"
)

var executeCLI = cli.Execute

func main() {
	run.SetBuildFactoryService(buildCLIRuntimeRunner)
	executeCLI()
}

func buildCLIRuntimeRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (run.RuntimeRunner, error) {
	runner, err := compose.InjectRuntimeRunner(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("initializer runtime runner missing")
	}
	return runner, nil
}
