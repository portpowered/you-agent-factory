// Package wire constructs the concrete application graph consumed by the
// process root. It does not select process modes or own component lifecycle.
package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

// BuildCLIRunner constructs the local runtime selected by the process root.
// Construction remains inert; initializer activates and shuts down the graph
// when the returned application is run.
func BuildCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (initializer.LocalRuntimeRunner, error) {
	return buildApplicationRunner(ctx, cfg, initializer.ModeCLI)
}

func buildApplicationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	mode initializer.Mode,
) (initializer.LocalRuntimeRunner, error) {
	runtimeCfg := service.RuntimeHostConfigFromFactoryService(cfg)
	if runtimeCfg != nil {
		copied := *runtimeCfg
		runtimeCfg = &copied
		if runtimeCfg.Logger == nil {
			runtimeCfg.Logger = zap.NewNop()
		}
		runtimeCfg.Clock = factory.EnsureClock(runtimeCfg.Clock)
	}
	graph, err := Build(ctx, Inputs{
		Config: runtimeCfg, MCPInput: strings.NewReader(""), MCPOutput: io.Discard,
	})
	if err != nil {
		return nil, err
	}
	application, err := initializer.NewApplication(mode, graph)
	if err != nil {
		if cleanupErr := graph.Close(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close rejected %s application graph: %w", mode, cleanupErr))
		}
		return nil, err
	}
	return application, nil
}
