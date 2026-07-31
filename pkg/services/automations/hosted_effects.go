package automations

import (
	"context"
	"net/http"
	"time"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	"go.uber.org/zap"
)

// HostedLinearCheckpointStore persists hosted Linear resume positions for
// process-edge injection.
type HostedLinearCheckpointStore = hostedsources.CheckpointStore

// HostedLinearHTTPDoer performs hosted Linear GraphQL network requests.
type HostedLinearHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// HostedRuntimePaths is the minimum runtime view needed to resolve a hosted
// source credential. Factory Definition runtime lookups satisfy this contract
// without becoming part of the Automations public dependency surface.
type HostedRuntimePaths interface {
	FactoryDir() string
	RuntimeBaseDir() string
}

// HostedLinearSecretResolver resolves hosted Linear credentials.
type HostedLinearSecretResolver func(
	context.Context,
	HostedRuntimePaths,
	string,
) (string, error)

// HostedLinearClock schedules hosted Linear poller waits.
type HostedLinearClock interface {
	After(time.Duration) <-chan time.Time
}

// NewHostedLinearCheckpointStore constructs atomic checkpoint persistence from
// an exact filesystem effect.
var NewHostedLinearCheckpointStore = hostedsources.NewCheckpointStore

// NewHostedLinearSecretResolver binds hosted Linear credential resolution
// effects.
func NewHostedLinearSecretResolver(
	getenv func(string) string,
	readFile func(string) ([]byte, error),
) HostedLinearSecretResolver {
	inner := hostedsources.NewSecretResolver(getenv, readFile)
	return func(ctx context.Context, runtimePaths HostedRuntimePaths, secretRef string) (string, error) {
		return inner(ctx, runtimePaths, secretRef)
	}
}

// HostedLinearDefaultRequestTimeout is the default hosted Linear HTTP timeout.
const HostedLinearDefaultRequestTimeout = hostedsources.DefaultRequestTimeout

// HostedSourcesFactory constructs the Automations-owned hosted-sources owner
// from explicit external effects.
type HostedSourcesFactory func(
	*zap.Logger,
	HostedLinearClock,
	HostedLinearHTTPDoer,
	HostedLinearSecretResolver,
	string,
) HostedPollers
