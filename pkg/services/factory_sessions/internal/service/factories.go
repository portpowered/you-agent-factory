package service

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// The factory roles below are consumed only while opening a Factory Session
// runtime. Keeping them here makes the dependency direction explicit: Wire
// supplies implementations, while this package owns the operation signature it
// needs. They are aliases to function signatures so the remaining legacy Wire
// providers can be cut over without an intermediate adapter graph.
type FactorySessionExecutionFactory = func(
	string,
	factorysessions.PersistencePolicy,
	providers.Service,
	factoryruntime.Clock,
	map[string]struct{},
	factoryruntime.JavaScriptWorkerSettings,
	*workers.MockWorkersConfig,
	[]operatorsettings.ACPIntegration,
	*zap.Logger,
) (durableexecution.Service, error)

// FactoryRuntimeRoot is the process-scoped Runtime capability. Factory
// Sessions supplies its activation operation per call while the root owns
// publication and lifecycle state.
type FactoryRuntimeRoot interface {
	factoryruntime.Service
	Activate(context.Context, factoryruntime.RuntimeActivationRequest, factoryruntime.RuntimeActivationOperation) (factoryruntime.RuntimeActivationResult, error)
	Deactivate(context.Context, factoryruntime.RuntimeDeactivationRequest) (factoryruntime.RuntimeDeactivationResult, error)
}

type DurableExecution struct {
	Service         durableexecution.Service
	WorkerSettings  *factoryruntime.JavaScriptWorkerSettings
	ACPIntegrations []operatorsettings.ACPIntegration
	OperatorModels  map[string]models.ModelOverlay
}

type DurableExecutionFactory func(
	factorydefinitions.RuntimeSelection,
	factorysessions.PersistencePolicy,
	string,
	string,
	operatorsettings.ResolvedDefaults,
	RuntimeRoot,
	factoryruntime.Clock,
	providers.Service,
	*workers.MockWorkersConfig,
	factorysessions.ProviderIdentityResolver,
) (DurableExecution, error)
