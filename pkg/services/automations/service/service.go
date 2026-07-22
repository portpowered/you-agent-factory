package service

import (
	"context"
	"errors"
	"time"

	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

var _ automations.Service = (*Service)(nil)

// Clock is the automation time source needed for scheduling and supervision.
type Clock interface {
	Now() time.Time
}

// Service supervises cron, poller, and watcher automation using injected collaborators.
type Service struct {
	loggerValue       *zap.Logger
	clock             Clock
	commandRunnerEdge workers.CommandRunner
	workflowID        string
	defaultFactoryDir string
	hostedPollers     automations.HostedPollers
	resolveTemplates  workers.TemplateFieldResolver
	executionPolicy   factorydefinitions.WorkstationExecutionPolicyService
}

// New constructs the automation service from explicit worker-sidecar
// dependencies.
func New(
	logger *zap.Logger,
	clock Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) *Service {
	return &Service{
		loggerValue:       logger,
		clock:             clock,
		commandRunnerEdge: commandRunner,
		workflowID:        workflowID,
		defaultFactoryDir: defaultFactoryDir,
		hostedPollers:     hostedPollers,
		resolveTemplates:  resolveTemplates,
		executionPolicy:   executionPolicy,
	}
}

// NewService constructs the Automations root contract for composition.
func NewService(
	logger *zap.Logger,
	clock Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) automations.Service {
	return New(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
	)
}

func (s *Service) logger() *zap.Logger {
	if s == nil || s.loggerValue == nil {
		return zap.NewNop()
	}
	return s.loggerValue
}

func (s *Service) commandRunner() workers.CommandRunner {
	if s != nil && s.commandRunnerEdge != nil {
		return s.commandRunnerEdge
	}
	return unavailableCommandRunner{}
}

type unavailableCommandRunner struct{}

func (unavailableCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("automation command runner is required")
}

func (s *Service) supervisorClock() clockwork.Clock {
	if s != nil {
		if clock, ok := s.clock.(clockwork.Clock); ok && clock != nil {
			return clock
		}
	}
	return clockwork.NewRealClock()
}

func (s *Service) pollerLogger(workstationName, workerName string) *zap.Logger {
	return s.logger().With(
		zap.String("workstation", workstationName),
		zap.String("worker", workerName),
	)
}
