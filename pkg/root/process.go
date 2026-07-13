package root

import (
	"context"
	"os"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/service"
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
	}, Dependencies{FactoryServiceBuilder: productionFactoryServiceBuilder})
}

func productionFactoryServiceBuilder(ctx context.Context, cfg *service.FactoryServiceConfig) (runcli.RuntimeRunner, error) {
	return wire.BuildCLIRunner(ctx, cfg)
}
