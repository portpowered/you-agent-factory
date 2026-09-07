package runtimeopening

import (
	"context"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	providers.Service,
	factoryruntime.Clock,
	map[string]struct{},
	factoryruntime.JavaScriptWorkerSettings,
	*workers.MockWorkersConfig,
	[]operatorsettings.ACPIntegration,
) (durableexecution.Service, error)

type ConductorInvocationWithProgressFactory = func(
	providers.Service,
	platformprocess.CommandRunner,
	workers.ProgressPublisher,
) (workers.InvocationExecutor, error)

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

type DurableExecution struct {
	Service         durableexecution.Service
	ACPIntegrations []operatorsettings.ACPIntegration
	OperatorModels  map[string]models.ModelOverlay
}

type DurableExecutionFactory func(
	factorydefinitions.RuntimeOpeningRequest,
	factorysessions.SessionRuntimeOpeningRequest,
	operatorsettings.ResolvedDefaults,
	RuntimeRoot,
	factoryruntime.Clock,
	providers.Service,
	*workers.MockWorkersConfig,
	FactorySessionExecutionFactory,
	factorysessions.ProviderIdentityResolver,
) (DurableExecution, error)
