package automations

import (
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	"go.uber.org/zap"
)

// HostedLinearCheckpointStore persists hosted Linear resume positions for
// process-edge injection.
type HostedLinearCheckpointStore = hostedsources.CheckpointStore

// HostedLinearHTTPDoer performs hosted Linear GraphQL network requests. The
// effect contract remains owned by the private hosted-sources implementation.
type HostedLinearHTTPDoer = hostedsources.HTTPDoer

// HostedRuntimePaths is the minimum runtime view needed to resolve a hosted
// source credential.
type HostedRuntimePaths = hostedsources.HostedRuntimePaths

// HostedLinearSecretResolver resolves hosted Linear credentials.
type HostedLinearSecretResolver = hostedsources.SecretResolver

// HostedLinearClock schedules hosted Linear poller waits.
type HostedLinearClock = hostedsources.Clock

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
