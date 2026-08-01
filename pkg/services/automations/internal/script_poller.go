package internal

import (
	"context"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// StartScriptPoller supervises one script poller workstation until ctx is canceled.
func (s *Service) StartScriptPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) {
	if s == nil || s.scriptPollers == nil {
		return
	}
	s.scriptPollers.StartScriptPoller(
		ctx,
		sidecars,
		runtimeCfg,
		workstation,
		workerDef,
		strings.TrimSpace(s.workflowIdentity(runtimeFactoryDir(runtimeCfg))),
		submitter,
	)
}

// RunScriptPoller executes one script poller command cycle and submits any parsed work request.
func (s *Service) RunScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) error {
	if s == nil || s.scriptPollers == nil {
		return nil
	}
	return s.scriptPollers.RunScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		workstation,
		workerDef,
		strings.TrimSpace(s.workflowIdentity(runtimeFactoryDir(runtimeCfg))),
		submitter,
	)
}

func runtimeFactoryDir(runtimeCfg interfaces.RuntimeConfigLookup) string {
	if runtimeCfg == nil {
		return ""
	}
	return runtimeCfg.FactoryDir()
}
