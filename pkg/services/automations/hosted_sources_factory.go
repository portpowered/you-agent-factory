package automations

import (
	"context"
	"sync"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type hostedPollersRootAdapter struct {
	inner hostedsources.HostedPollers
}

func (h hostedPollersRootAdapter) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter HostedWorkSubmitter,
) error {
	return h.inner.StartLinearPoller(ctx, sidecars, runtimeConfig, workstation, worker, hostedsources.WorkSubmitter(submitter))
}

func (h hostedPollersRootAdapter) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter HostedWorkSubmitter,
) error {
	return h.inner.ValidateLinearPoller(runtimeConfig, workstation, worker, hostedsources.WorkSubmitter(submitter))
}

// NewHostedSourcesFactory returns the runtime-opening factory that composes
// hosted Linear polling through the Automations-owned hosted_sources package.
func NewHostedSourcesFactory(checkpointStore HostedLinearCheckpointStore) HostedSourcesFactory {
	store := checkpointStore
	return func(
		logger *zap.Logger,
		clock workers.HostedPollerClock,
		httpClient workers.HostedPollerHTTPDoer,
		secretResolver workers.HostedPollerSecretResolver,
		linearEndpoint string,
	) HostedPollers {
		return hostedPollersRootAdapter{inner: hostedsourceswire.NewHostedPollers(
			logger,
			clock,
			httpClient,
			secretResolver,
			linearEndpoint,
			store,
		)}
	}
}

var _ HostedPollers = hostedPollersRootAdapter{}
