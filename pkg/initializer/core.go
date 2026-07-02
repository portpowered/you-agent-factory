package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

// Core is the normalized runtime graph composed before transport facades attach.
type Core = runtimehost.Core

// BuildCore loads factory configuration and composes the normalized runtime graph
// through pkg/initializer as the canonical composition entrypoint.
func BuildCore(ctx context.Context, cfg *Config) (*Core, error) {
	return buildCore(ctx, cfg)
}
