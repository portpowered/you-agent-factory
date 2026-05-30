// Package main is the entry point for the agent-factory CLI.
package main

import (
	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/run"
)

var executeCLI = cli.Execute

func main() {
	run.SetBuildFactoryService(run.FactoryServiceBuilderFromService(compose.InjectFactoryService))
	executeCLI()
}
