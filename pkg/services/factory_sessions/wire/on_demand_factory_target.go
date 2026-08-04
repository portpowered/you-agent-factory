package wire

import (
	"context"

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

// TargetExecutionService is the narrow, owner-published Factory Sessions
// capability for asynchronous target start, captured-session invocation,
// captured-turn cancellation, and target close. It uses the exact request,
// result, and error vocabulary factorysessions.Service already publishes for
// these four operations (StartAsync, InvokeFactorySession, Cancel,
// CloseFactorySession), so Service itself satisfies this interface
// structurally -- see the var _ assertion below. It is declared in this wire
// package rather than the factorysessions root so the root keeps exactly one
// named interface (Service); a peer that only needs to start, invoke,
// cancel, and close a selected target depends on this narrower interface
// instead, and can never reach an unrelated aggregate-service operation.
type TargetExecutionService interface {
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error)
	Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
}

// factorysessions.Service satisfies TargetExecutionService structurally, and
// so does the on-demand activation this package constructs: both compile-time
// assertions keep the two interfaces from silently drifting apart.
var (
	_ TargetExecutionService = (factorysessions.Service)(nil)
	_ TargetExecutionService = (*OnDemandFactoryTargetService)(nil)
)

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
