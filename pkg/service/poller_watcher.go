package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

type workRequestSubmitter func(context.Context, interfaces.WorkRequest) error

func (fs *FactoryService) startSchedulerSidecarsForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter workRequestSubmitter,
) error {
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return nil
	}

	workersScheduler, err := fs.requireWorkersScheduler()
	if err != nil {
		return err
	}
	return workersScheduler.StartSchedulerSidecarsForRuntime(
		ctx,
		sidecars,
		workersservice.RuntimeSidecarsInput{
			FactoryDir: factoryDir,
			FactoryCfg: factoryCfg,
			RuntimeCfg: runtimeCfg,
			Submitter:  workersservice.WorkRequestSubmitter(submitter),
		},
	)
}

func (fs *FactoryService) requireWorkersScheduler() (*workersservice.Service, error) {
	if fs == nil || fs.workersScheduler == nil {
		return nil, fmt.Errorf("worker sidecar owner is not initialized: construct FactoryService through the runtime initializer")
	}
	return fs.workersScheduler, nil
}

func (fs *FactoryService) submitCronTick(
	ctx context.Context,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	workersScheduler, err := fs.requireWorkersScheduler()
	if err != nil {
		return err
	}
	runtimeCfg := fs.currentRuntimeConfig()
	workflowIdentity := ""
	runtimeLookup := interfaces.FirstRuntimeWorkstationLookup(runtimeCfg)
	submitter := fs.currentRuntimeSubmitter()
	if runtimeCfg != nil {
		workflowIdentity = workersScheduler.WorkflowIdentityForFactoryDir(runtimeCfg.FactoryDir())
	}
	return workersScheduler.SubmitCronTick(
		ctx,
		runtimeLookup,
		workflowIdentity,
		workersservice.WorkRequestSubmitter(submitter),
		ws,
		firedAt,
	)
}
