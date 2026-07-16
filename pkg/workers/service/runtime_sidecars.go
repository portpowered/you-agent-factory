package service

import (
	"context"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// RuntimeSidecarsInput carries explicit runtime-host collaborators for worker-side
// poller and cron supervision.
type RuntimeSidecarsInput struct {
	FactoryDir string
	FactoryCfg *interfaces.FactoryConfig
	RuntimeCfg interfaces.RuntimeConfigLookup
	Submitter  WorkRequestSubmitter
}

// StartSchedulerSidecarsForRuntime supervises configured poller and cron workstations
// until ctx is canceled. Runtime hosts attach both schedulers through this narrow API.
func (s *Service) StartSchedulerSidecarsForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	input RuntimeSidecarsInput,
) error {
	if input.FactoryCfg == nil || input.RuntimeCfg == nil || sidecars == nil || input.Submitter == nil {
		return nil
	}
	if err := s.ValidatePollersForRuntime(input.FactoryCfg, input.RuntimeCfg, input.Submitter); err != nil {
		return err
	}

	s.StartCronWatchersForRuntime(
		ctx,
		sidecars,
		input.FactoryDir,
		input.FactoryCfg,
		input.RuntimeCfg,
		input.Submitter,
	)
	return s.StartPollersForRuntime(
		ctx,
		sidecars,
		input.FactoryCfg,
		input.RuntimeCfg,
		input.Submitter,
	)
}
