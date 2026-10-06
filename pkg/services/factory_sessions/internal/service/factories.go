package service

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

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
	Release         func(context.Context) error
	WorkerSettings  *factoryruntime.JavaScriptWorkerSettings
	ACPIntegrations []operatorsettings.ACPIntegration
	OperatorModels  map[string]models.ModelOverlay
}
