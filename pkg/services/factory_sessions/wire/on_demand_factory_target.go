package wire

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/ondemandtarget"
	"go.uber.org/zap"
)

// FactoryTargetRuntimeResolver is re-published here so a caller outside the
// factory_sessions tree can declare a field or parameter of this type
// without importing the internal ondemandtarget package directly, matching
// how this package already re-publishes RuntimeOpeningFactory and its peers.
type FactoryTargetRuntimeResolver = ondemandtarget.RuntimeResolver

// OnDemandFactoryTargetService is re-published here for the same reason;
// the actual operational implementation lives in the owning service's
// internal/ondemandtarget package, not this wire package, matching this
// package's own established RuntimeOpeningFactory convention (a construction
// alias plus a thin wrapper constructor, never operational implementation).
type OnDemandFactoryTargetService = ondemandtarget.Service

// The on-demand activation this package constructs satisfies the
// owner-published factorysessions.TargetExecutionService structurally; this
// compile-time assertion keeps the two from silently drifting apart. The
// interface itself lives on the factory_sessions root (see
// target_execution_contract.go there), not this wire package, so a peer
// service or transport depends on the actual owning service's published
// contract directly instead of a wire-only construction alias.
var _ factorysessions.TargetExecutionService = (*OnDemandFactoryTargetService)(nil)

// NewOnDemandFactoryTargetService constructs the on-demand Factory Sessions
// activation over the given already-wired RuntimeOpeningFactory. Construction
// alone performs no I/O and opens no runtime.
func NewOnDemandFactoryTargetService(
	factory *RuntimeOpeningFactory,
	effects RuntimeOpeningExternalEffects,
	resolve FactoryTargetRuntimeResolver,
	generateID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*OnDemandFactoryTargetService, error) {
	return ondemandtarget.New(factory, effects, resolve, generateID, logger)
}
