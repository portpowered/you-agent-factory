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
	// attempt or provider identity. It is a cheap, deliberately racy
	// pre-filter (the turn can end at any moment after this call returns);
	// callers that need a truthful accept/reject outcome must use TryCancel,
	// which re-derives liveness atomically at the instant it acts instead of
	// trusting an earlier Cancelable observation.
	Cancelable(id providers.ID, attemptID string) bool

	// TryCancel atomically determines whether id/attemptID names the exact
	// live session/prompt turn and, only if so, delivers a session/cancel
	// protocol notification to it and blocks (bounded by ctx) until the turn
	// returns. accepted is true only when the turn's real recorded outcome
	// was that cancellation -- never merely because a matching identity was
	// observed a moment earlier -- so a natural completion racing this call
	// is reported as accepted=false, not a false positive. A non-nil err
	// reports a genuine delivery failure or ctx ending before the turn's
	// outcome could be observed; both are distinguishable from
	// accepted=false, err=nil (unsupported/lost-the-race) and from each
	// other (errors.Is against the returned error identifies which).
	TryCancel(ctx context.Context, id providers.ID, attemptID string) (accepted bool, err error)
}
