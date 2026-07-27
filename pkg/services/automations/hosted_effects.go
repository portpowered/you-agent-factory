package automations

import (
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// HostedLinearCheckpointStore persists hosted Linear resume positions for
// process-edge injection.
type HostedLinearCheckpointStore = hostedsources.CheckpointStore

// HostedLinearHTTPDoer performs hosted Linear GraphQL network requests.
type HostedLinearHTTPDoer = workers.HostedPollerHTTPDoer

// HostedLinearSecretResolver resolves hosted Linear credentials.
type HostedLinearSecretResolver = workers.HostedPollerSecretResolver

// HostedLinearClock schedules hosted Linear poller waits.
type HostedLinearClock = workers.HostedPollerClock

// NewHostedLinearCheckpointStore constructs atomic checkpoint persistence from
// an exact filesystem effect.
var NewHostedLinearCheckpointStore = hostedsources.NewCheckpointStore

// NewHostedLinearSecretResolver binds hosted Linear credential resolution
// effects.
var NewHostedLinearSecretResolver = hostedsources.NewSecretResolver

// HostedLinearDefaultRequestTimeout is the default hosted Linear HTTP timeout.
const HostedLinearDefaultRequestTimeout = hostedsources.DefaultRequestTimeout

// HostedSourcesFactory constructs the Automations-owned hosted-sources owner
// from explicit external effects.
type HostedSourcesFactory func(
	*zap.Logger,
	workers.HostedPollerClock,
	workers.HostedPollerHTTPDoer,
	workers.HostedPollerSecretResolver,
	string,
) HostedPollers
