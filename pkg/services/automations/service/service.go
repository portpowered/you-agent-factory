package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
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
	reconciler        reconciliation.Service
	scriptPollers     scriptpollers.Service
	schedulerMu       sync.Mutex
	schedulerSources  map[automations.SourceIdentity]*schedulerSource
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
	service := &Service{
		loggerValue:       logger,
		clock:             clock,
		commandRunnerEdge: commandRunner,
		workflowID:        workflowID,
		defaultFactoryDir: defaultFactoryDir,
		hostedPollers:     hostedPollers,
		resolveTemplates:  resolveTemplates,
		executionPolicy:   executionPolicy,
		schedulerSources:  make(map[automations.SourceIdentity]*schedulerSource),
	}
	service.reconciler = service.newSchedulerReconciler()
	service.scriptPollers = service.newScriptPollers()
	return service
}

func (s *Service) newScriptPollers() scriptpollers.Service {
	return scriptpollerswire.NewService(scriptpollers.Dependencies{
		Logger:           s.pollerLogger,
		Clock:            s.supervisorClock,
		CommandRunner:    s.commandRunner,
		ResolveTemplates: s.resolveTemplates,
		ExecutionPolicy:  s.executionPolicy,
	})
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
) *Service {
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

// Root returns the inert published Automation operations backed by the same
// reconciliation owner used for runtime scheduler supervision.
func (s *Service) Root() automations.Root {
	if s == nil || s.reconciler == nil {
		return automations.Root{}
	}
	return automations.Root{Operations: s}
}

func (s *Service) Reconcile(
	ctx context.Context,
	request automations.ReconcileRequest,
) (automations.ReconcileResult, error) {
	return s.reconciler.Reconcile(ctx, request)
}

func (s *Service) StartSource(
	ctx context.Context,
	request automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	return s.reconciler.StartSource(ctx, request)
}

func (s *Service) StopSource(
	ctx context.Context,
	request automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	return s.reconciler.StopSource(ctx, request)
}

func (s *Service) WaitSource(
	ctx context.Context,
	request automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	return s.reconciler.WaitSource(ctx, request)
}

func (s *Service) SourceStatus(
	ctx context.Context,
	request automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	return s.reconciler.SourceStatus(ctx, request)
}

func (s *Service) GetStatus(
	ctx context.Context,
	request automations.GetStatusRequest,
) (automations.GetStatusResult, error) {
	return s.reconciler.GetStatus(ctx, request)
}

func (s *Service) GetCursor(
	ctx context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	return s.reconciler.GetCursor(ctx, request)
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
