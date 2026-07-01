package service

import (
	"context"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

func (fs *FactoryService) startPollerWatchersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter workRequestSubmitter,
) {
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return
	}

	fs.workersSchedulerService().StartPollersForRuntime(
		ctx,
		sidecars,
		factoryCfg,
		runtimeCfg,
		workersservice.WorkRequestSubmitter(submitter),
	)
}

func (fs *FactoryService) workersSchedulerService() *workersservice.Service {
	clock := clockwork.NewRealClock()
	if fs != nil {
		if supervisorClock, ok := fs.clock.(clockwork.Clock); ok && supervisorClock != nil {
			clock = supervisorClock
		}
	}
	var runner workers.CommandRunner = workers.ExecCommandRunner{}
	if fs != nil && fs.coordinatorPolicy().commandRunnerOverride != nil {
		runner = fs.coordinatorPolicy().commandRunnerOverride
	}
	logger := zap.NewNop()
	if fs != nil && fs.logger != nil {
		logger = fs.logger
	}
	hosted := hostedworkers.Config{}
	if fs != nil {
		hosted = fs.hostedWorkers
	}
	return workersservice.New(workersservice.Config{
		Logger:               logger,
		Clock:                clock,
		CommandRunner:        runner,
		WorkflowID:             fs.coordinatorPolicy().workflowID,
		DefaultFactoryDir:      fs.coordinatorPolicy().dir,
		HostedHTTPClient:       hosted.HTTPClient,
		HostedSecretResolver:   hosted.SecretResolver,
		HostedLinearEndpoint:   hosted.LinearEndpoint,
	})
}

const (
	cronSourceTag          = interfaces.TimeWorkTagKeySource
	cronWorkstationTag     = interfaces.TimeWorkTagKeyCronWorkstation
	cronSubmissionNamePref = "cron:"
)

type workRequestSubmitter func(context.Context, interfaces.WorkRequest) error

func (fs *FactoryService) startCronWatchersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	submitter workRequestSubmitter,
) {
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || submitter == nil {
		return
	}

	fs.workersSchedulerService().StartCronWatchersForRuntime(
		ctx,
		sidecars,
		factoryDir,
		factoryCfg,
		runtimeCfg,
		workersservice.WorkRequestSubmitter(submitter),
	)
}

func (fs *FactoryService) submitCronTick(
	ctx context.Context,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	runtimeCfg := fs.currentRuntimeConfig()
	workflowIdentity := ""
	runtimeLookup := interfaces.FirstRuntimeWorkstationLookup(runtimeCfg)
	submitter := fs.currentRuntimeSubmitter()
	if runtimeCfg != nil {
		workflowIdentity = fs.cronWorkflowIdentity(runtimeCfg.FactoryDir())
	}
	return fs.workersSchedulerService().SubmitCronTick(
		ctx,
		runtimeLookup,
		workflowIdentity,
		workersservice.WorkRequestSubmitter(submitter),
		ws,
		firedAt,
	)
}

func (fs *FactoryService) cronWorkflowIdentity(factoryDir string) string {
	return fs.workersSchedulerService().WorkflowIdentityForFactoryDir(factoryDir)
}
