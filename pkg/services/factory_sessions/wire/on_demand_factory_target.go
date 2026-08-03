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
