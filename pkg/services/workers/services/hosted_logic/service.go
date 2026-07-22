package hostedworkers

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic/linear"
	"go.uber.org/zap"
)

// Service is the provider-specific hosted polling implementation exposed
// through automations.HostedPollers.
type Service struct {
	logger         *zap.Logger
	clock          Clock
	httpClient     hostedlinear.HTTPDoer
	secretResolver hostedlinear.SecretResolver
	checkpoints    hostedlinear.CheckpointStore
	linearEndpoint string
}

// New constructs hosted polling without starting its lifecycle.
func New(
	logger *zap.Logger,
	clock Clock,
	httpClient hostedlinear.HTTPDoer,
	secretResolver hostedlinear.SecretResolver,
	linearEndpoint string,
	checkpointStores ...hostedlinear.CheckpointStore,
) *Service {
	var checkpoints hostedlinear.CheckpointStore
	if len(checkpointStores) > 0 {
		checkpoints = checkpointStores[0]
	}
	return &Service{
		logger: defaultLogger(logger), clock: clock, httpClient: httpClient,
		secretResolver: secretResolver, checkpoints: checkpoints, linearEndpoint: defaultLinearEndpoint(linearEndpoint),
	}
}

func (s *Service) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return StartLinearPoller(
		ctx, sidecars,
		s.logger, s.clock, s.httpClient, s.secretResolver, s.checkpoints, s.linearEndpoint,
		runtimeConfig, workstation, worker, Submitter(submitter),
	)
}

func (s *Service) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	_, err := NewLinearPoller(
		s.logger, s.clock, s.httpClient, s.secretResolver, s.checkpoints, s.linearEndpoint,
		runtimeConfig, workstation, worker, Submitter(submitter),
	)
	return err
}

var _ automations.HostedPollers = (*Service)(nil)
