// Package leases defines the parent-private Models Runtime Host leases owner.
package leases

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// DefaultLeaseTTL is the detached lease lifetime issued on successful acquire.
const DefaultLeaseTTL = time.Minute

// SlotFacts captures Runtime Host readiness and configured capacity for one
// scoped model slot without exposing private host internals.
type SlotFacts struct {
	Readiness models.ReadinessState
	Capacity  int
}

// SlotFactsProvider supplies host-owned readiness and capacity facts for lease
// acquisition. Runtime Host implements this port; leases construction requires
// a non-nil provider but does not start supervision from it.
type SlotFactsProvider interface {
	SlotFacts(context.Context, models.RuntimeScopeRef, string) (SlotFacts, error)
}

// UnconfiguredSlotFacts reports runtime-not-ready for every slot until Runtime
// Host wires a live facts adapter during lease call-site cutover.
type UnconfiguredSlotFacts struct{}

func (UnconfiguredSlotFacts) SlotFacts(
	context.Context,
	models.RuntimeScopeRef,
	string,
) (SlotFacts, error) {
	return SlotFacts{}, models.ErrHostRuntimeNotReady
}

// Service owns capacity reservation and holder lifecycle over Runtime Host slots.
// Peers reach acquire, release, and get lease behavior only through the singular
// process-scoped Models root; this interface stays parent-private.
type Service interface {
	AcquireModelLease(
		context.Context,
		models.AcquireModelLeaseRequest,
	) (models.AcquireModelLeaseResult, error)
	GetModelLease(
		context.Context,
		models.GetModelLeaseRequest,
	) (models.GetModelLeaseResult, error)
	ReleaseModelLease(
		context.Context,
		models.ReleaseModelLeaseRequest,
	) (models.ReleaseModelLeaseResult, error)
}
