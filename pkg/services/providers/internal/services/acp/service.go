// Package acp declares the parent-private Agent Client Protocol service.
package acp

import (
	"context"

	acpsdk "github.com/coder/acp-go-sdk"
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

	// NegotiatedCapabilities returns the exact AgentCapabilities id's daemon
	// negotiated the last time its ACP initialize handshake completed
	// successfully, without starting, blocking on, or otherwise causing any
	// new provider connection or side effect. ok is false until id's daemon
	// has completed at least one successful handshake; once true, the last
	// successfully negotiated value is retained even while the daemon is
	// temporarily disconnected, so a caller can never regress from a known
	// truth back to an unknown or default claim.
	NegotiatedCapabilities(id providers.ID) (acpsdk.AgentCapabilities, bool)

	// Claim atomically captures the exact live execution generation named by
	// id/attemptID, without any other side effect, when this service
	// currently has an established session/prompt turn in flight for it --
	// the only ACP protocol state a session/cancel notification can
	// truthfully target. ok is false before the session exists, after the
	// turn has already returned, and for any other attempt or provider
	// identity. The returned Generation is an opaque capability: TryCancel
	// delivers only to the exact generation it names, so a control that has
	// claimed one generation can never be redirected to a later generation
	// that reuses the identical id/attemptID strings, even if this attempt
	// completes and a new one binds that identity before TryCancel runs.
	Claim(id providers.ID, attemptID string) (generation Generation, ok bool)

	// TryCancel delivers a session/cancel protocol notification to the exact
	// generation named by generation (as previously captured by Claim) and
	// blocks (bounded by ctx) until it observes that generation's real
	// recorded terminal outcome. accepted is true only when the generation's
	// real recorded outcome was that cancellation -- never merely because
	// Claim observed it live a moment earlier -- so a natural completion
	// racing this call, or a generation that has already ended by the time
	// this call runs, is reported as accepted=false, not a false positive. A
	// non-nil err reports a genuine delivery failure or ctx ending before the
	// outcome could be observed; both are distinguishable from
	// accepted=false, err=nil (unsupported/lost-the-race/already-terminal)
	// and from each other (errors.Is against the returned error identifies
	// which).
	TryCancel(ctx context.Context, generation Generation) (accepted bool, err error)
}

// ContinuationService is the private ACP extension that receives a session
// reference only after the Providers root has validated a Continue request.
type ContinuationService interface {
	Service
	Continue(context.Context, providers.ID, providers.ExecuteRequest, providers.SessionRef) (providers.ExecuteResult, error)
}

// Generation is an opaque capability naming one exact live ACP execution
// generation, captured by Claim at the instant it observed that generation
// live. Provider and attempt identity strings alone can never rediscover or
// replace the generation a Generation value names: this is what lets a
// claimed control retain authority over only the exact generation it was
// claimed from, even after that generation ends and a later execution reuses
// the same canonical provider and attempt ID.
type Generation any
