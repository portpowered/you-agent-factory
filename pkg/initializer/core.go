package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
)

// Core is the normalized runtime graph composed before transport facades attach.
type Core = runtimehost.Core

// BuildCore loads factory configuration and composes the normalized runtime graph
// through pkg/initializer as the canonical composition entrypoint.
func BuildCore(ctx context.Context, cfg *Config) (*Core, error) {
	if err := service.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	root, err := service.ResolveFactoryServiceRoot(cfg)
	if err != nil {
		return nil, err
	}
	load, err := service.LoadFactoryConfigForCompose(cfg, root)
	if err != nil {
		return nil, err
	}
	clock := service.ServiceClockForCompose(cfg, load)
	collaborators := service.NewFactoryServiceCollaborators(cfg, clock, root.BaseLogger, service.NewFactorySessionsRegistry())
	return service.ComposeFactoryCore(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		service.NewHostedWorkersConfig(cfg, root.BaseLogger, clock),
	)
}
