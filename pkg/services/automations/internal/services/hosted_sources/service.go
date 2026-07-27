// Package hostedsources owns Automations hosted Linear observation, secret
// resolution for observation, poll/restart/checkpoint behavior, observation
// normalization, and Work-admission commanding. Callers outside Automations
// consume the outer Automations service root instead of this parent-private
// package.
package hostedsources

import (
	"context"
	"sync"

	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Clock is the exact time effect used by hosted-source supervision.
type Clock = workers.HostedPollerClock

// HTTPDoer performs the Linear adapter's external network request.
type HTTPDoer = workers.HostedPollerHTTPDoer

// HostedRuntimePaths is the minimum runtime view needed to resolve a hosted
// worker credential.
type HostedRuntimePaths = workers.HostedRuntimePaths

// SecretResolver resolves the external credential used by the Linear adapter.
type SecretResolver = workers.HostedPollerSecretResolver

// CheckpointStore persists hosted Linear resume positions.
type CheckpointStore = hostedlinear.CheckpointStore

// CheckpointFileSystem is the exact host filesystem effect required by the
// hosted Linear checkpoint adapter.
type CheckpointFileSystem = hostedlinear.CheckpointFileSystem

// NewCheckpointStore constructs atomic checkpoint persistence from an exact
// filesystem effect. Production selection belongs to Wire.
func NewCheckpointStore(files CheckpointFileSystem) (CheckpointStore, error) {
	return hostedlinear.NewCheckpointStore(files)
}

// NewSecretResolver binds the exact environment and filesystem effects used to
// resolve hosted-worker credentials.
func NewSecretResolver(
	getenv func(string) string,
	readFile func(string) ([]byte, error),
) SecretResolver {
	return hostedlinear.NewSecretResolver(getenv, readFile)
}

// DefaultRequestTimeout is the hosted Linear HTTP client timeout used by Wire.
const DefaultRequestTimeout = hostedlinear.DefaultRequestTimeout

// DefaultEndpoint is the hosted Linear GraphQL endpoint used when none is configured.
const DefaultEndpoint = hostedlinear.DefaultEndpoint

// WorkSubmitter admits one normalized Work Request produced by a hosted poller.
type WorkSubmitter func(context.Context, work.WorkRequest) error

// HostedPollers supervises hosted Linear polling for Automations.
type HostedPollers interface {
	StartLinearPoller(
		context.Context,
		*sync.WaitGroup,
		interfaces.RuntimeConfigLookup,
		interfaces.FactoryWorkstationConfig,
		*interfaces.FactoryWorkerConfig,
		WorkSubmitter,
	) error
	ValidateLinearPoller(
		interfaces.RuntimeConfigLookup,
		interfaces.FactoryWorkstationConfig,
		*interfaces.FactoryWorkerConfig,
		WorkSubmitter,
	) error
}
