package wire

import (
	"context"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// hostedPollersRootAdapter translates the owner-private hosted-sources
// submitter vocabulary into the Automations construction vocabulary.
type hostedPollersRootAdapter struct {
	inner hostedsources.HostedPollers
}

func (h hostedPollersRootAdapter) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.StartLinearPoller(ctx, sidecars, runtimeConfig, workstation, worker, hostedsources.WorkSubmitter(submitter))
}

func (h hostedPollersRootAdapter) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.ValidateLinearPoller(runtimeConfig, workstation, worker, hostedsources.WorkSubmitter(submitter))
}

// NewHostedSourcesFactory returns the runtime-opening factory that composes
// hosted Linear polling through the Automations-owned hosted_sources package.
func NewHostedSourcesFactory(checkpointStore automations.HostedLinearCheckpointStore) automations.HostedSourcesFactory {
	store := checkpointStore
	return func(
		logger *zap.Logger,
		clock automations.HostedLinearClock,
		httpClient automations.HostedLinearHTTPDoer,
		secretResolver automations.HostedLinearSecretResolver,
		linearEndpoint string,
	) automations.HostedPollers {
		return hostedPollersRootAdapter{inner: hostedsourceswire.NewHostedPollers(
			logger,
			clock,
			httpClient,
			adaptSecretResolver(secretResolver),
			linearEndpoint,
			store,
		)}
	}
}

func adaptSecretResolver(resolver automations.HostedLinearSecretResolver) hostedsources.SecretResolver {
	if resolver == nil {
		return nil
	}
	return func(ctx context.Context, runtimePaths hostedsources.HostedRuntimePaths, secretRef string) (string, error) {
		return resolver(ctx, runtimePaths, secretRef)
	}
}

var _ automations.HostedPollers = hostedPollersRootAdapter{}
