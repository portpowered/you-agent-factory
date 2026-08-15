package providers

import "context"

// Service is the singular cross-service Providers root authority. Peer packages
// depend on this one named interface for Providers-owned catalog enumeration,
// identity/selection authority, availability/capability facts, and one normalized execution attempt rather
// than Workers provider registry/conductor types or concrete adapter packages.
// Providers owns exactly one native attempt per Execute call; callers own
// selection, retry, throttle, and scheduling policy.
type Service interface {
	// ListProviders returns detached catalog descriptors for every known
	// provider, including availability and capability facts. Unavailable or
	// prerequisite-blocked providers remain listed with their catalog facts.
	ListProviders(context.Context, ListProvidersRequest) (ListProvidersResult, error)
	// GetProvider returns one detached catalog descriptor for a Providers-owned
	// provider identity. Invalid identity fails with ErrInvalidID, unknown
	// identity fails with ErrUnknownProvider, and blocked availability or
	// missing prerequisite facts fail with ErrProviderUnavailable. Static
	// catalog requirements remain selectable but are reported as unverified
	// until a readiness probe supplies current facts.
	GetProvider(context.Context, GetProviderRequest) (GetProviderResult, error)
	// ResolveIdentity canonicalizes one Providers-owned ID or accepted alias
	// without starting execution.
	ResolveIdentity(context.Context, ResolveIdentityRequest) (ResolveIdentityResult, error)
	// ResolveSelection applies Providers-owned selection precedence and returns
	// one canonical provider identity.
	ResolveSelection(context.Context, ResolveSelectionRequest) (ResolveSelectionResult, error)
	// ValidatePrerequisites verifies that one canonical provider is currently
	// selectable.
	ValidatePrerequisites(context.Context, ValidatePrerequisitesRequest) error
	// Execute performs exactly one normalized provider attempt. Invalid request
	// identity fails with ErrInvalidID. Attempt failures return typed
	// Providers-owned errors such as ErrExecuteCancelled, ErrExecuteTimeout, and
	// ErrExecuteFailed that peers can branch on with errors.Is / errors.As.
	// Successful results and typed failures may carry an optional detached
	// SessionRef for the provider session observed during the attempt.
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
	// ControlAttempt requests pause, cancel, or terminate for one identified
	// provider attempt. Invalid provider, attempt, or action input fails with
	// ErrInvalidID or ErrInvalidControlRequest before any outcome is produced.
	// Valid requests return a typed completed or unsupported outcome as a
	// successful result; unsupported is not encoded as an error.
	ControlAttempt(context.Context, ControlAttemptRequest) (ControlAttemptResult, error)
}

// ContinuationService is the optional Providers capability for exact-session
// continuation. Keeping it separate from Service lets ordinary provider test
// doubles and integrations adopt continuation incrementally without changing
// the one-attempt execution contract.
type ContinuationService interface {
	Continue(context.Context, ContinueRequest) (ContinueResult, error)
}

// Continue routes an exact-session continuation when the supplied Providers
// root implements the optional capability. Roots without that capability
// report the same explicit unsupported outcome as a provider that cannot
// resume the requested session; they never fall back to Execute.
func Continue(
	ctx context.Context,
	service Service,
	request ContinueRequest,
) (ContinueResult, error) {
	continuation, ok := service.(ContinuationService)
	if !ok {
		return ContinueResult{
			Reference: request.Reference.Clone(),
			Outcome:   ContinuationOutcomeUnsupported,
		}, nil
	}
	return continuation.Continue(ctx, request)
}
