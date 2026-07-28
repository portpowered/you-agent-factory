package internal

import (
	"context"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// StartHostedLinearPoller supervises one hosted Linear poller workstation until ctx is canceled.
func (s *Service) StartHostedLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) error {
	if submitter == nil {
		return nil
	}
	if s == nil || s.hostedPollers == nil {
		return nil
	}
	return s.hostedPollers.StartLinearPoller(
		ctx,
		sidecars,
		runtimeCfg,
		workstation,
		workerDef,
		automations.HostedWorkSubmitter(submitter),
	)
}

func (s *Service) validateHostedLinearPoller(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) error {
	if s == nil || s.hostedPollers == nil {
		return nil
	}
	return s.hostedPollers.ValidateLinearPoller(
		runtimeCfg, workstation, workerDef, automations.HostedWorkSubmitter(submitter),
	)
}
