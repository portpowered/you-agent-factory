package runtimeopening

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// The factory roles below are consumed only while opening a Factory Session
// runtime. Keeping them here makes the dependency direction explicit: Wire
// supplies implementations, while this package owns the operation signature it
// needs. They are aliases to function signatures so the remaining legacy Wire
// providers can be cut over without an intermediate adapter graph.
type WorkFactory = func(work.RuntimeResolver) work.Service

type FactorySessionExecutionFactory = func(
	string,
	factorysessions.PersistencePolicy,
	workers.Provider,
	factoryruntime.Clock,
	map[string]struct{},
	factoryruntime.JavaScriptWorkerSettings,
	*workers.MockWorkersConfig,
	[]operatorsettings.ACPIntegration,
) (durableexecution.Service, error)

type ConductorInvocationWithProgressFactory = func(
	providers.Service,
	workers.CommandRunner,
	workers.PTYAllocator,
	workers.ProgressPublisher,
) (workers.InvocationExecutor, error)

type WorkersRuntimeFactory = func(
	roles.CurrentRuntimeResolver,
	models.Service,
	models.RuntimeScopeRef,
	workers.CommandRunner,
	workers.CommandRunner,
	workers.ProgressPublisher,
	workers.PTYAllocator,
	*zap.Logger,
	bool,
	string,
	string,
	string,
	*bool,
	workers.Provider,
	func() time.Time,
	work.ContentMaterializer,
	[]operatorsettings.ACPIntegration,
) (workers.RuntimeService, error)

// RuntimeRootFactory constructs the inert process-scoped Factory Runtime root
// with the opening operation supplied by this owner. The root constructor is
// selected by the canonical process composition package, while the activation
// callback remains owned by Factory Sessions because it assembles the session
// product handoff.
type FactoryRuntimeRoot interface {
	factoryruntime.Service
	Activate(context.Context, factoryruntime.RuntimeActivationRequest) (factoryruntime.RuntimeActivationResult, error)
	Deactivate(context.Context, factoryruntime.RuntimeDeactivationRequest) (factoryruntime.RuntimeDeactivationResult, error)
}

type RuntimeRootFactory func(factoryruntime.RuntimeActivationOperation) (FactoryRuntimeRoot, error)

type WorkersLocalRuntimeHooksFactory = func() workers.LocalRuntimeHooks

type DurableExecution struct {
	Service         durableexecution.Service
	ACPIntegrations []operatorsettings.ACPIntegration
}

type DurableExecutionFactory func(
	factorydefinitions.RuntimeOpeningRequest,
	factorysessions.SessionRuntimeOpeningRequest,
	operatorsettings.ResolvedDefaults,
	RuntimeRoot,
	factoryruntime.Clock,
	workers.Provider,
	*workers.MockWorkersConfig,
	FactorySessionExecutionFactory,
	factorysessions.ProviderIdentityResolver,
) (DurableExecution, error)

type WorkerExecutionFactory func(
	factoryruntime.RuntimeOpeningRequest,
	workers.RuntimeOpeningRequest,
	factoryruntime.Clock,
	*zap.Logger,
	workers.CommandRunner,
	workers.CommandRunner,
	workers.ProgressPublisher,
	workers.PTYAllocator,
	workers.Provider,
	roles.CurrentRuntimeResolver,
	models.Service,
	models.RuntimeScopeRef,
	work.Service,
	WorkersRuntimeFactory,
	[]operatorsettings.ACPIntegration,
	func(workers.RuntimeService) bool,
) (workers.RuntimeService, workers.SessionBuildFactory, error)
