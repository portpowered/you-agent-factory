// Package leases defines the parent-private Models Runtime Host leases owner.
package leases

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// DefaultLeaseTTL is the detached lease lifetime issued on successful acquire.
const DefaultLeaseTTL = time.Minute

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

// LeaseRevoker is an optional host-failure capability. Runtime Host uses it
// to invalidate active capacity when a supervised process crashes; ordinary
// lease callers do not depend on this lifecycle operation.
type LeaseRevoker interface {
	RevokeModelLeases(models.RuntimeScopeRef, string)
}
