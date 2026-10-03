// Package wire is the Automations service composition boundary.
//
// Focused providers construct one inert implementation each. Canonical Wire
// supplies completed behaviors to the operation owner; runtime activation
// allocates scoped state and resources through those already-injected owners.
package wire

import (
	"context"
	"sync"

	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	cursorscopeswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/wire"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	fswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	sourcelifecyclewire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// CursorScopes exposes the private recovery owner to canonical composition.
type CursorScopes = cursorscopes.CursorScopes

type SourceLifecycle = sourcelifecycle.SourceLifecycle
type Reconciliation = reconciliation.Service
type ScriptPollers = scriptpollers.Service
type Cron = cron.Service
type FilesystemWatchers = filesystemwatchers.Service
type Clock = clockwork.Clock

// NewSourceLifecycle constructs one independent owner from completed behaviors.
// C08 includes the operator-authorized canonical cron execution-policy input.
func NewSourceLifecycle(logger *zap.Logger, clock Clock, scriptPollers ScriptPollers,
	cronService Cron, filesystemWatchers FilesystemWatchers, hostedPollers automations.HostedPollers,
	cursors CursorScopes, executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) SourceLifecycle {
	return sourcelifecyclewire.NewService(logger, clock, scriptPollers, cronService,
		filesystemWatchers, hostedPollers, cursors, executionPolicy)
}

func NewReconciliation(lifecycle SourceLifecycle) Reconciliation {
	return reconciliationwire.NewService(lifecycle)
}

// CursorPersistenceFileSystem is the cursor owner's exact external effect.
type CursorPersistenceFileSystem = cursorscopeswire.CursorPersistenceFileSystem

// NewCursorScopes constructs the scoped recovery owner without filesystem IO.
func NewCursorScopes(files CursorPersistenceFileSystem) CursorScopes {
	return cursorscopeswire.NewService(files)
}

// Owner is the completed Automations operation implementation.
type Owner = automationinternal.Service

func NewScriptPollers(logger *zap.Logger, clock Clock, commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService, cursors CursorScopes) ScriptPollers {
	return scriptpollerswire.NewService(logger, clock, commandRunner, resolveTemplates, executionPolicy, cursors)
}
func NewCron() Cron                             { return cronwire.NewService() }
func NewFilesystemWatchers() FilesystemWatchers { return fswire.NewService() }

// NewService stores already-completed owners and selected persistence behavior.
func NewService(logger *zap.Logger, clock Clock, lifecycle SourceLifecycle,
	reconciler Reconciliation, scriptPollers ScriptPollers, cronService Cron,
	filesystemWatchers FilesystemWatchers, hostedPollers automations.HostedPollers,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService, cursors CursorScopes,
	persistRuntimeCursors bool, workflowID, defaultFactoryDir, cursorBaseDir string) *Owner {
	return automationinternal.New(logger, clock, lifecycle, reconciler, scriptPollers,
		cronService, filesystemWatchers, hostedPollers, executionPolicy, cursors, persistRuntimeCursors, workflowID, defaultFactoryDir, cursorBaseDir)
}

// NewRoot projects the already-constructed owner without assembly or effects.
func NewRoot(service *Owner) automations.Root { return service.Root() }

type hostedPollersRootAdapter struct {
	inner hostedsources.HostedPollers
}

func (h hostedPollersRootAdapter) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.StartLinearPoller(
		ctx,
		sidecars,
		runtimeConfig,
		workstation,
		worker,
		hostedsources.WorkSubmitter(submitter),
	)
}

func (h hostedPollersRootAdapter) ValidateLinearPoller(
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.ValidateLinearPoller(
		runtimeConfig,
		workstation,
		worker,
		hostedsources.WorkSubmitter(submitter),
	)
}

// NewHostedPollers constructs one inert hosted-source owner from selected effects.
func NewHostedPollers(
	logger *zap.Logger,
	clock automations.HostedLinearClock,
	httpClient automations.HostedLinearHTTPDoer,
	secretResolver automations.HostedLinearSecretResolver,
	linearEndpoint string,
	checkpointStore automations.HostedLinearCheckpointStore,
) automations.HostedPollers {
	return hostedPollersRootAdapter{inner: hostedsourceswire.NewHostedPollers(
		logger,
		clock,
		httpClient,
		adaptSecretResolver(secretResolver),
		linearEndpoint,
		checkpointStore,
	)}
}

func adaptSecretResolver(
	resolver automations.HostedLinearSecretResolver,
) hostedsources.SecretResolver {
	if resolver == nil {
		return nil
	}
	return func(
		ctx context.Context,
		runtimePaths hostedsources.HostedRuntimePaths,
		secretRef string,
	) (string, error) {
		return resolver(ctx, runtimePaths, secretRef)
	}
}
