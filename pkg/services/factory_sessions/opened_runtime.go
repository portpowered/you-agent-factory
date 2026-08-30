package factorysessions

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	ProgressFragmentKind  = workers.ProgressFragmentKind
	ResponseFragmentKind  = workers.ResponseFragmentKind
	CompletedFragmentKind = workers.CompletedFragmentKind
	FailedFragmentKind    = workers.FailedFragmentKind
)

type ProgressFragment = workers.ProgressFragment

type ProgressPublisher = workers.ProgressPublisher

// SessionRuntimeOpeningRequest contains Factory Session identity,
// persistence, startup, and hosting values for one runtime.
type SessionRuntimeOpeningRequest struct {
	// FactorySessionID correlates the opened runtime with its owning Factory
	// Session. Empty values use the process's primary session alias.
	FactorySessionID string
	// CanonicalSessionID is the preallocated runtime identity for an automatic
	// default recording and its runtime metrics. It is distinct from the public
	// FactorySessionID alias.
	CanonicalSessionID string
	// CanonicalSessionIDGenerated distinguishes the opener's preallocation from
	// a caller-supplied canonical identity when the request crosses the Runtime
	// activation boundary.
	CanonicalSessionIDGenerated bool
	PersistencePolicy           PersistencePolicy
	BackendScopeID              string
	SystemConfigHome            string
	SystemConfigPath            string
	WorkFile                    string
	Host                        RuntimeHostRequest
}

// RuntimeOpeningRequest is the Factory Sessions operation input assembled from
// bounded owner requests. The complete service graph remains injected; this
// value carries only per-runtime customer and process selections.
type RuntimeOpeningRequest struct {
	FactoryDefinition   factorydefinitions.RuntimeOpeningRequest
	FactoryRuntime      factoryruntime.RuntimeOpeningRequest
	FactorySession      SessionRuntimeOpeningRequest
	Workers             workers.RuntimeOpeningRequest
	Recordings          recordings.RuntimeOpeningRequest
	ModelCacheDirectory string
	OperatorDefaults    operatorsettings.ResolvedDefaults
}
