package service

import (
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/workers"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

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
