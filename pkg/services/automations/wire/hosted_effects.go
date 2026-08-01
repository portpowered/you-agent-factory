package wire

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
)

// NewHostedLinearCheckpointStore constructs atomic checkpoint persistence from
// an exact filesystem effect. Checkpoint selection belongs to application
// Wire, not the Automations peer root.
var NewHostedLinearCheckpointStore = hostedsources.NewCheckpointStore

// NewHostedLinearSecretResolver binds hosted Linear credential resolution to
// explicit environment and filesystem effects.
func NewHostedLinearSecretResolver(
	getenv func(string) string,
	readFile func(string) ([]byte, error),
) automations.HostedLinearSecretResolver {
	inner := hostedsources.NewSecretResolver(getenv, readFile)
	return func(ctx context.Context, runtimePaths automations.HostedRuntimePaths, secretRef string) (string, error) {
		return inner(ctx, runtimePaths, secretRef)
	}
}
