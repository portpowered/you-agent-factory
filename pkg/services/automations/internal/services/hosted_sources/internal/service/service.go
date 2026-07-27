package service

import (
	"context"
	"sync"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// Service is the Automations-owned hosted Linear polling implementation.
type Service struct {
	logger         *zap.Logger
	clock          hostedsources.Clock
	httpClient     hostedlinear.HTTPDoer
	secretResolver hostedlinear.SecretResolver
	checkpoints    hostedlinear.CheckpointStore
	linearEndpoint string
}

// New constructs hosted polling without starting its lifecycle.
func New(
	logger *zap.Logger,
	clock hostedsources.Clock,
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
	submitter Submitter,
) error {
	return StartLinearPoller(
		ctx, sidecars,
		s.logger, s.clock, s.httpClient, s.secretResolver, s.checkpoints, s.linearEndpoint,
		runtimeConfig, workstation, worker, submitter,
	)
}

func (s *Service) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter Submitter,
) error {
	_, err := NewLinearPoller(
		s.logger, s.clock, s.httpClient, s.secretResolver, s.checkpoints, s.linearEndpoint,
		runtimeConfig, workstation, worker, submitter,
	)
	return err
}
