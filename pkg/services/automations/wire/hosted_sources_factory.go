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

// NewHostedPollers composes hosted Linear polling inside the Automations owner.
func NewHostedPollers(
	logger *zap.Logger,
	clock automations.HostedLinearClock,
	httpClient automations.HostedLinearHTTPDoer,
	secretResolver automations.HostedLinearSecretResolver,
	linearEndpoint string,
	checkpointStore automations.HostedLinearCheckpointStore,
) automations.HostedPollers {
	return hostedPollersRootAdapter{inner: hostedsourceswire.NewHostedPollers(
		logger,
		clock,
		httpClient,
		adaptSecretResolver(secretResolver),
		linearEndpoint,
		checkpointStore,
	)}
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
