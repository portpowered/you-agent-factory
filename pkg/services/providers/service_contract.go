package providers

import (
	"context"
	"fmt"
	"strings"
)

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
}

// ControlService is the optional Providers capability for controlling one
// in-flight attempt. Keeping it separate from Service lets ordinary provider
// roots adopt control without breaking existing execution-only doubles.
type ControlService interface {
	// ControlAttempt requests pause, cancel, or terminate for one identified
	// provider attempt. Invalid provider, attempt, or action input fails with
	// ErrInvalidID or ErrInvalidControlRequest before any outcome is produced.
	// Valid requests return a typed completed or unsupported outcome as a
	// successful result; unsupported is not encoded as an error.
	ControlAttempt(context.Context, ControlAttemptRequest) (ControlAttemptResult, error)
}

// ControlAttempt routes an attempt-control request when the supplied Providers
// root implements the optional capability. Roots without it return the typed
// unsupported outcome and never claim that a control action completed.
func ControlAttempt(
	ctx context.Context,
	service Service,
	request ControlAttemptRequest,
) (ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return ControlAttemptResult{}, err
	}
	control, ok := service.(ControlService)
	if !ok {
		return ControlAttemptResult{
			Provider:  request.Provider,
			AttemptID: request.AttemptID,
			Action:    request.Action,
			Outcome:   ControlOutcomeUnsupported,
		}, nil
	}
	return control.ControlAttempt(ctx, request)
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
	if err := request.Validate(); err != nil {
		return ContinueResult{}, err
	}
	continuation, ok := service.(ContinuationService)
	if !ok {
		return ContinueResult{
			Reference: request.Reference.Clone(),
			Outcome:   ContinuationOutcomeUnsupported,
		}, nil
	}
	return continuation.Continue(ctx, request)
}

// ContinueReferenceRequest carries the detached continuation vocabulary to
// the Providers root. It is intentionally separate from ContinueRequest so
// Workers and Runtime do not need to construct or inspect a provider-owned
// SessionRef while forwarding an opaque continuation.
type ContinueReferenceRequest struct {
	Reference ContinuationRef
	Attempt   ExecuteRequest
}

// ContinueReferenceResult is the detached result of one opaque continuation
// request. Reference echoes the canonical provider identity and exact session
// kind/id while retaining any caller-supplied external reference.
type ContinueReferenceResult struct {
	Reference ContinuationRef
	Outcome   ContinuationOutcome
	Result    ExecuteResult
}

// ContinueReference resolves and routes one opaque continuation through the
// Providers root. It never falls back to Execute: a missing continuation
// capability is returned as the explicit unsupported outcome.
func ContinueReference(
	ctx context.Context,
	service Service,
	request ContinueReferenceRequest,
) (ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return ContinueReferenceResult{}, continuationReferenceFailure(
			ContinuationFailureKindInvalid,
			err.Error(),
			request.Reference,
		)
	}
	if service == nil {
		return ContinueReferenceResult{}, continuationReferenceFailure(
			ContinuationFailureKindInvalid,
			"Providers service is required",
			request.Reference,
		)
	}

	canonical, err := service.ResolveIdentity(ctx, ResolveIdentityRequest{
		Identity: reference.Provider.String(),
	})
	if err != nil {
		return ContinueReferenceResult{}, continuationReferenceFailure(
			ContinuationFailureKindForeign,
			err.Error(),
			request.Reference,
		)
	}
	if err := canonical.ID.Validate(); err != nil {
		return ContinueReferenceResult{}, continuationReferenceFailure(
			ContinuationFailureKindForeign,
			err.Error(),
			request.Reference,
		)
	}
	reference.Provider = canonical.ID

	attempt := request.Attempt.Clone()
	if strings.TrimSpace(attempt.Provider.String()) == "" {
		attempt.Provider = canonical.ID
	} else {
		attemptIdentity, resolveErr := service.ResolveIdentity(ctx, ResolveIdentityRequest{
			Identity: attempt.Provider.String(),
		})
		if resolveErr != nil || attemptIdentity.ID != canonical.ID {
			message := "attempt provider does not match continuation provider"
			if resolveErr != nil {
				message = resolveErr.Error()
			}
			return ContinueReferenceResult{}, continuationReferenceFailure(
				ContinuationFailureKindForeign,
				message,
				request.Reference,
			)
		}
		attempt.Provider = canonical.ID
	}

	continued, err := Continue(ctx, service, ContinueRequest{
		Reference: reference,
		Attempt:   attempt,
	})
	if err != nil {
		return ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if strings.TrimSpace(continuedReference.Provider.String()) == "" {
		continuedReference = reference
	}
	resultReference := ContinuationRefFromSession(continuedReference)
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return ContinueReferenceResult{
		Reference: resultReference,
		Outcome:   continued.Outcome,
		Result:    continued.Result,
	}, nil
}

func continuationReferenceFailure(
	kind ContinuationFailureKind,
	message string,
	ref ContinuationRef,
) ContinuationFailure {
	normalized := ref.Normalize()
	return ContinuationFailure{
		Kind:    kind,
		Message: message,
		Reference: SessionRef{
			Provider: ID(normalized.Provider),
			Kind:     normalized.Kind,
			ID:       firstContinuationIdentity(normalized),
		},
	}
}

func firstContinuationIdentity(ref ContinuationRef) string {
	if value := strings.TrimSpace(ref.ProviderSessionID); value != "" {
		return value
	}
	return strings.TrimSpace(ref.ExternalRef)
}

// Clone returns a detached continuation-reference result.
func (result ContinueReferenceResult) Clone() ContinueReferenceResult {
	clone := result
	clone.Reference = result.Reference.Clone()
	clone.Result = result.Result.Clone()
	return clone
}

// String returns a bounded identity useful for diagnostics without exposing
// provider-specific request material.
func (ref ContinuationRef) String() string {
	normalized := ref.Normalize()
	return fmt.Sprintf("%s/%s/%s", normalized.Provider, normalized.Kind, firstContinuationIdentity(normalized))
}
