// Package internal owns Automations operations over completed injected behaviors.
package internal

import (
	"context"
	"sync"

	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

var _ automations.Service = (*Service)(nil)

type WorkRequestSubmitter = automations.WorkRequestSubmitter
type Clock = clockwork.Clock

// Service stores completed behaviors; runtime activation allocates only scoped state.
type Service struct {
	loggerValue           *zap.Logger
	clock                 Clock
	workflowID            string
	defaultFactoryDir     string
	hostedPollers         automations.HostedPollers
	executionPolicy       factorydefinitions.WorkstationExecutionPolicyService
	persistRuntimeCursors bool
	cursors               cursorscopes.CursorScopes
	reconciler            reconciliation.Service
	scriptPollers         scriptpollers.Service
	cursorScope           scriptpollers.CursorScope
	cron                  cron.Service
	filesystemWatchers    filesystemwatchers.Service
	lifecycle             sourcelifecycle.SourceLifecycle
	runtimeMu             sync.Mutex
	runtimes              map[string]*runtimeInstance
	runtimeActivating     map[string]struct{}
}

// New stores completed owners and the caller's selected runtime persistence policy.
func New(logger *zap.Logger, clock Clock, lifecycle sourcelifecycle.SourceLifecycle,
	reconciler reconciliation.Service, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService, cursors cursorscopes.CursorScopes, persistRuntimeCursors bool,
	workflowID, defaultFactoryDir, cursorBaseDir string,
) *Service {
	return &Service{loggerValue: logger, clock: clock, lifecycle: lifecycle,
		reconciler: reconciler, scriptPollers: scriptPollers, cron: cronService,
		filesystemWatchers: filesystemWatchers, hostedPollers: hostedPollers,
		executionPolicy: executionPolicy, cursors: cursors, persistRuntimeCursors: persistRuntimeCursors,
		workflowID: workflowID, defaultFactoryDir: defaultFactoryDir, cursorScope: scriptpollers.CursorScope{BaseDir: cursorBaseDir},
		runtimes: make(map[string]*runtimeInstance), runtimeActivating: make(map[string]struct{})}
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
