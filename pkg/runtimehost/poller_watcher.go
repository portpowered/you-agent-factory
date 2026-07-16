package runtimehost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

type workRequestSubmitter func(context.Context, interfaces.WorkRequest) error

type workerSidecarOwner interface {
	StartSchedulerSidecarsForRuntime(context.Context, *sync.WaitGroup, workersservice.RuntimeSidecarsInput) error
	WorkflowIdentityForFactoryDir(string) string
	SubmitCronTick(context.Context, interfaces.RuntimeWorkstationLookup, string, workersservice.WorkRequestSubmitter, interfaces.FactoryWorkstationConfig, time.Time) error
}

func (fs *Host) startSchedulerSidecarsForRuntime(
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

func (fs *Host) requireWorkersScheduler() (workerSidecarOwner, error) {
	if fs == nil || fs.workersScheduler == nil {
		return nil, fmt.Errorf("worker sidecar owner is not initialized: construct Host through the runtime initializer")
	}
	return fs.workersScheduler, nil
}
