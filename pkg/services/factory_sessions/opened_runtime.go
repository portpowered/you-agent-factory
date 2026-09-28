package factorysessions

import (
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
