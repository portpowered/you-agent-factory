// Package wire constructs the Automations hosted_sources subservice.
package wire

import (
	"context"
	"sync"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/service"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

type hostedPollers struct {
	inner *hostedservice.Service
}

func (h hostedPollers) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter hostedsources.WorkSubmitter,
) error {
	return h.inner.StartLinearPoller(ctx, sidecars, runtimeConfig, workstation, worker, hostedservice.Submitter(submitter))
}

func (h hostedPollers) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter hostedsources.WorkSubmitter,
) error {
	return h.inner.ValidateLinearPoller(runtimeConfig, workstation, worker, hostedservice.Submitter(submitter))
}

// NewHostedPollers constructs an inert hosted-sources owner from explicit effects.
func NewHostedPollers(
	logger *zap.Logger,
	clock hostedsources.Clock,
	httpClient hostedsources.HTTPDoer,
	secretResolver hostedsources.SecretResolver,
	linearEndpoint string,
	checkpointStores ...hostedsources.CheckpointStore,
) hostedsources.HostedPollers {
	return hostedPollers{inner: hostedservice.New(
		logger,
		clock,
		httpClient,
		secretResolver,
		linearEndpoint,
		checkpointStores...,
	)}
}

var _ hostedsources.HostedPollers = hostedPollers{}
