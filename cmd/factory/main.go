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
	transport, err := compose.InjectCLITransport(ctx, cfg)
	if err != nil {
		return nil, err
	}
	runner := transport.Runner()
	if runner == nil {
		return nil, errors.New("initializer CLI transport missing runtime runner")
	}
	return runner, nil
}
