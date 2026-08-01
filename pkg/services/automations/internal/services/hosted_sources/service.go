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
)

// Clock is the exact time effect used by hosted-source supervision.
type Clock = hostedlinear.Clock

// HTTPDoer performs the Linear adapter's external network request.
type HTTPDoer = hostedlinear.HTTPDoer

// HostedRuntimePaths is the minimum runtime view needed to resolve a hosted
// source credential.
type HostedRuntimePaths = hostedlinear.RuntimePaths

// SecretResolver resolves the external credential used by the Linear adapter.
type SecretResolver = hostedlinear.SecretResolver

// CheckpointStore persists hosted Linear resume positions.
type CheckpointStore = hostedlinear.CheckpointStore

// CheckpointFileSystem is the exact host filesystem effect required by the
// hosted Linear checkpoint adapter.
type CheckpointFileSystem = hostedlinear.CheckpointFileSystem

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
