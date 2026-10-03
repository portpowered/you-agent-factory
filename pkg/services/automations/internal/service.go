// Package internal composes Automations runtime sidecars for script pollers,
// hosted pollers, filesystem watchers, and cron Work generation. Script
// command/source polling is owned by internal/services/script_pollers and
// reached only through this Automations root. Wire supplies explicit
// submitters, clocks, loggers, cancellation, and configuration. Use
// StartSchedulerSidecarsForRuntime as the unified runtime entrypoint for poller
// and cron supervision.
package internal

import (
	"context"
	"strings"
	"sync"

	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	cursorscopeswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/wire"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

var _ automations.Service = (*Service)(nil)

// WorkRequestSubmitter submits parsed poller or cron work requests into the runtime.
type WorkRequestSubmitter = automations.WorkRequestSubmitter

// Clock is the required automation scheduler. Owner Wire selects its
// scheduling capabilities before constructing behavior or runtime resources.
type Clock = clockwork.Clock

// Service supervises cron, poller, and watcher automation using injected collaborators.
type Service struct {
	loggerValue        *zap.Logger
	clock              Clock
	commandRunnerEdge  platformprocess.CommandRunner
	workflowID         string
	defaultFactoryDir  string
	hostedPollers      automations.HostedPollers
	resolveTemplates   workers.TemplateFieldResolver
	executionPolicy    factorydefinitions.WorkstationExecutionPolicyService
	cursorFileSystem   cursorscopeswire.CursorPersistenceFileSystem
	reconciler         reconciliation.Service
	scriptPollers      scriptpollers.Service
	cursors            cursorscopes.CursorScopes
	cursorScope        scriptpollers.CursorScope
	cron               cron.Service
	filesystemWatchers filesystemwatchers.Service
	schedulerMu        sync.Mutex
	schedulerSources   map[automations.SourceIdentity]*schedulerSource
	runtimeMu          sync.Mutex
	runtimes           map[string]*runtimeInstance
	runtimeActivating  map[string]struct{}
}

// New constructs the automation service from explicit worker-sidecar
// dependencies.
func New(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
) *Service {
	return NewWithCursorFileSystem(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
		nil,
		cronService,
		filesystemWatchers,
	)
}

// NewWithCursorFileSystem constructs an owner with the production cursor
// persistence effect. Direct owner-local callers can use New and retain the
// in-memory recorder.
func NewWithCursorFileSystem(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorFileSystem cursorscopeswire.CursorPersistenceFileSystem,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
) *Service {
	cursors := cursorscopeswire.NewService(cursorFileSystem)
	pollers := scriptpollerswire.NewService(logger, clock, commandRunner, resolveTemplates, executionPolicy, cursors)
	return newService(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
		cursorFileSystem,
		cronService,
		filesystemWatchers,
		pollers,
		cursors,
	)
}

func newService(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorFileSystem cursorscopeswire.CursorPersistenceFileSystem,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
	pollers scriptpollers.Service,
	cursors cursorscopes.CursorScopes,
) *Service {
	service := &Service{
		loggerValue:        logger,
		clock:              clock,
		commandRunnerEdge:  commandRunner,
		workflowID:         workflowID,
		defaultFactoryDir:  defaultFactoryDir,
		hostedPollers:      hostedPollers,
		resolveTemplates:   resolveTemplates,
		executionPolicy:    executionPolicy,
		cursorFileSystem:   cursorFileSystem,
		cron:               cronService,
		filesystemWatchers: filesystemWatchers,
		scriptPollers:      pollers,
		cursors:            cursors,
		schedulerSources:   make(map[automations.SourceIdentity]*schedulerSource),
		runtimes:           make(map[string]*runtimeInstance),
		runtimeActivating:  make(map[string]struct{}),
	}
	service.reconciler = service.newSchedulerReconciler()
	// Preserve the memory-only owner fixture when no persistence edge is supplied.
	// A blank process-root directory also remains memory-backed, never CWD-relative.
	if cursorFileSystem != nil {
		service.cursorScope.BaseDir = strings.TrimSpace(defaultFactoryDir)
	}
	return service
}

// NewService constructs the Automations root contract for composition.
func NewService(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
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
		cronService,
		filesystemWatchers,
	)
}

// Root returns the inert published Automation operations backed by the same
// reconciliation owner used for runtime scheduler supervision.
func (s *Service) Root() automations.Root {
	if s == nil || s.reconciler == nil {
		return automations.Root{}
	}
	return automations.Root{Operations: s, Lifecycle: s, Runtime: s}
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
	if s != nil && s.scriptPollers != nil && scriptpollers.IsScriptPollerInstanceID(request.InstanceID) {
		if result, handled, err := s.getCursorFromActiveRuntime(ctx, request); handled {
			return result, err
		}
		return s.scriptPollers.GetCursorForScope(ctx, s.cursorScope, request)
	}
	return s.reconciler.GetCursor(ctx, request)
}
