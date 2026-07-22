package service

import (
	"context"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// StartSchedulerSidecarsForRuntime supervises configured poller and cron workstations
// until ctx is canceled. Runtime hosts attach both schedulers through this narrow API.
func (s *Service) StartSchedulerSidecarsForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryConfig *interfaces.FactoryConfig,
	runtimeConfig interfaces.RuntimeConfigLookup,
	submitter automations.WorkRequestSubmitter,
) error {
	if factoryConfig == nil || runtimeConfig == nil || sidecars == nil || submitter == nil {
		return nil
	}
	if err := s.ValidatePollersForRuntime(factoryConfig, runtimeConfig, submitter); err != nil {
		return err
	}

	s.StartCronWatchersForRuntime(
		ctx,
		sidecars,
		factoryDir,
		factoryConfig,
		runtimeConfig,
		submitter,
	)
	return s.StartPollersForRuntime(
		ctx,
		sidecars,
		factoryConfig,
		runtimeConfig,
		submitter,
	)
}
