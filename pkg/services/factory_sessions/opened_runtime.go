package factorysessions

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// SessionRuntimeOpeningRequest contains Factory Session identity,
// persistence, startup, and hosting values for one runtime.
type SessionRuntimeOpeningRequest struct {
	PersistencePolicy PersistencePolicy
	BackendScopeID    string
	SystemConfigHome  string
	SystemConfigPath  string
	WorkFile          string
	Host              RuntimeHostRequest
}

// RuntimeOpeningRequest is the Factory Sessions operation input assembled from
// bounded owner requests. The complete service graph remains injected; this
// value carries only per-runtime customer and process selections.
type RuntimeOpeningRequest struct {
	FactoryDefinition factorydefinitions.RuntimeOpeningRequest
	FactoryRuntime    factoryruntime.RuntimeOpeningRequest
	FactorySession    SessionRuntimeOpeningRequest
	Workers           workers.RuntimeOpeningRequest
	Recordings        recordings.RuntimeOpeningRequest
	Models            models.RuntimeOpeningRequest
	OperatorDefaults  operatorsettings.ResolvedDefaults
}
