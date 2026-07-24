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

var (
	_ automations.Service          = (*Service)(nil)
	_ automations.RuntimeScheduler = (*Service)(nil)
	_ automations.Root             = (*Service)(nil)
)

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
) automations.Root {
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

// Ready reports that the concrete Automations root is available for published
// contract slices.
func (s *Service) Ready(context.Context, automations.ReadyRequest) (automations.ReadyResult, error) {
	if s == nil {
		return automations.ReadyResult{}, &automations.Error{
			Op:   "Ready",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	return automations.ReadyResult{Ready: true}, nil
}

// Reconcile is the published Automations root reconcile slice. Nested
// reconciliation ownership remains an IMP-AUTO packet; this additive stub keeps
// the concrete root aligned with the published Service contract.
func (s *Service) Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error) {
	if s == nil {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	return automations.ReconcileResult{}, nil
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
