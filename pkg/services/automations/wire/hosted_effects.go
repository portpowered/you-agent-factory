package wire

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
)

// NewHostedLinearCheckpointStore constructs atomic checkpoint persistence from
// an exact filesystem effect. Checkpoint selection belongs to application
// Wire, not the Automations peer root.
var NewHostedLinearCheckpointStore = hostedsourceswire.NewCheckpointStore

// NewHostedLinearSecretResolver binds hosted Linear credential resolution to
// explicit environment and filesystem effects.
func NewHostedLinearSecretResolver(
	getenv func(string) string,
	readFile func(string) ([]byte, error),
) automations.HostedLinearSecretResolver {
	inner := hostedsourceswire.NewSecretResolver(getenv, readFile)
	return func(ctx context.Context, runtimePaths automations.HostedRuntimePaths, secretRef string) (string, error) {
		return inner(ctx, runtimePaths, secretRef)
	}
}
