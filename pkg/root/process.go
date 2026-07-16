package root

import (
	"context"
	"os"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/wire"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
)

// Run executes one process input and translates its terminal outcome without
// printing an additional diagnostic.
func Run(input Input, dependencies Dependencies) int {
	if err := ExecuteWithDependencies(input, dependencies); err != nil {
		return ExitFailure
	}
	return ExitSuccess
}

// Main is the production process entrypoint used by cmd/factory.
func Main() int {
	return Run(Input{
		Args:    os.Args,
		Env:     os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: context.Background(),
	}, Dependencies{})
}

type productionGraphBuilder struct {
	buildGraph   wire.ProcessGraphBuilder
	dependencies wire.ProcessGraphDependencies
}

func (builder productionGraphBuilder) Build(ctx context.Context, request GraphRequest) (*ApplicationGraph, error) {
	if builder.buildGraph == nil {
		return wire.BuildProcessGraph(ctx, request.Startup, request.Policy)
	}
	return builder.buildGraph(ctx, request.Startup, request.Policy, builder.dependencies)
}

type productionInitializer struct {
	initialize wire.ProcessInitializer
}

func (production productionInitializer) Run(ctx context.Context, initialization Initialization) error {
	if production.initialize == nil {
		production.initialize = initializer.RunProcess
	}
	return production.initialize(ctx, initialization.Graph)
}
