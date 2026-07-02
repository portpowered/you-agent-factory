package service

import (
	"context"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

type workRequestSubmitter func(context.Context, interfaces.WorkRequest) error

func (fs *FactoryService) startSchedulerSidecarsForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter workRequestSubmitter,
) {
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return
	}

	fs.requireWorkersScheduler().StartSchedulerSidecarsForRuntime(
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

func (fs *FactoryService) requireWorkersScheduler() *workersservice.Service {
	if fs == nil {
		return workersservice.New(workersservice.Config{})
	}
	if fs.workersScheduler != nil {
		return fs.workersScheduler
	}
	if fs.cfg != nil {
		return NewWorkersSchedulerService(fs.cfg, fs.clock, fs.logger, fs.hostedWorkers)
	}
	clock := clockwork.NewRealClock()
	if supervisorClock, ok := fs.clock.(clockwork.Clock); ok && supervisorClock != nil {
		clock = supervisorClock
	}
	runner := workers.CommandRunner(workers.ExecCommandRunner{})
	if fs.coordinatorPolicy().commandRunnerOverride != nil {
		runner = fs.coordinatorPolicy().commandRunnerOverride
	}
	logger := zap.NewNop()
	if fs.logger != nil {
		logger = fs.logger
	}
	hosted := fs.hostedWorkers
	return workersservice.New(workersservice.Config{
		Logger:               logger,
		Clock:                clock,
		CommandRunner:        runner,
		WorkflowID:           fs.coordinatorPolicy().workflowID,
		DefaultFactoryDir:    fs.coordinatorPolicy().dir,
		HostedHTTPClient:     hosted.HTTPClient,
		HostedSecretResolver: hosted.SecretResolver,
		HostedLinearEndpoint: hosted.LinearEndpoint,
	})
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
	return fs.requireWorkersScheduler().SubmitCronTick(
		ctx,
		runtimeLookup,
		workflowIdentity,
		workersservice.WorkRequestSubmitter(submitter),
		ws,
		firedAt,
	)
}

func (fs *FactoryService) cronWorkflowIdentity(factoryDir string) string {
	return fs.requireWorkersScheduler().WorkflowIdentityForFactoryDir(factoryDir)
}

// NewWorkersSchedulerService constructs the workers scheduling collaborator for
// runtime-host poller and cron supervision.
func NewWorkersSchedulerService(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) *workersservice.Service {
	supervisorClock := clockwork.NewRealClock()
	if clock != nil {
		if clockworkClock, ok := clock.(clockwork.Clock); ok && clockworkClock != nil {
			supervisorClock = clockworkClock
		}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	runner := workers.CommandRunner(workers.ExecCommandRunner{})
	workflowID := ""
	defaultFactoryDir := ""
	if cfg != nil {
		if cfg.CommandRunnerOverride != nil {
			runner = cfg.CommandRunnerOverride
		}
		workflowID = cfg.WorkflowID
		defaultFactoryDir = cfg.Dir
	}
	return workersservice.New(workersservice.Config{
		Logger:               logger,
		Clock:                supervisorClock,
		CommandRunner:        runner,
		WorkflowID:           workflowID,
		DefaultFactoryDir:    defaultFactoryDir,
		HostedHTTPClient:     hostedWorkers.HTTPClient,
		HostedSecretResolver: hostedWorkers.SecretResolver,
		HostedLinearEndpoint: hostedWorkers.LinearEndpoint,
	})
}
