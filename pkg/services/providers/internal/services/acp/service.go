// Package acp declares the parent-private Agent Client Protocol service.
package acp

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service owns configured ACP peers and their protocol lifecycle. Execution
// delegates ACP attempts here instead of constructing protocol adapters.
type Service interface {
	providers.Lifecycle
	Configure(context.Context, []providers.ACPIntegration) error
	Integrations() []providers.ACPIntegration
	Resolve(providers.ID) (providers.ID, bool)
	Execute(context.Context, providers.ID, providers.ExecuteRequest) (providers.ExecuteResult, error)

	// Cancelable reports, without any side effect, whether id/attemptID names
	// the exact attempt this service currently has an established session/
	// prompt turn in flight for -- the only ACP protocol state a session/
	// cancel notification can truthfully target. It answers false before the
	// session exists, after the turn has already returned, and for any other
	// attempt or provider identity.
	Cancelable(id providers.ID, attemptID string) bool

	// Cancel delivers a session/cancel protocol notification to id/
	// attemptID's exact in-flight session/prompt turn and blocks until that
	// turn has returned. It is a harmless no-op if attemptID is no longer
	// the live turn (for example, the turn finished naturally between a
	// caller's Cancelable check and this call); callers that need a
	// truthful accept/reject signal must call Cancelable first under the
	// same exclusive claim.
	Cancel(ctx context.Context, id providers.ID, attemptID string) error
}
