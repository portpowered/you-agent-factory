// Package execution defines the parent-private Providers execution service.
package execution

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service performs one provider-neutral adapter attempt for an explicit
// Providers-owned request.
type Service interface {
	Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
}

// Attempt is the private adapter seam for one normalized provider invocation.
// It deliberately carries no retry, fallback, scheduling, or throttle policy.
type Attempt func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)

// Registration binds one canonical Providers identity to one private adapter
// attempt.
type Registration struct {
	Provider providers.ID
	Attempt  Attempt
}
