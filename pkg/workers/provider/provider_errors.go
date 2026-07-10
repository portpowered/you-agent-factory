package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ProviderError is the shared normalized provider failure contract. Provider
// implementations should return this typed error so executor, pause, and
// customer-messaging logic can make deterministic decisions without parsing raw
// provider output at every call site.
type ProviderError struct {
	Family          interfaces.WorkFailureFamily
	Type            interfaces.WorkFailureType
	Message         string
	ProviderSession *interfaces.ProviderSessionMetadata
	Diagnostics     *interfaces.WorkDiagnostics
	Cause           error
}

// ProviderFailureResult is the pure output of provider failure parsing. It
// deliberately carries only the canonical reason and customer-visible message;
// runtime policy is derived from Reason when the result crosses into execution.
type ProviderFailureResult struct {
	Reason  interfaces.WorkFailureType
	Message string
}

func NewProviderError(errorType interfaces.WorkFailureType, message string, cause error) *ProviderError {
	return NewProviderErrorFromResult(ProviderFailureResult{
		Reason:  errorType,
		Message: message,
	}, cause)
}

// NewProviderErrorFromResult turns a pure parse result into the normalized
// execution error while deriving all runtime policy from its canonical reason.
func NewProviderErrorFromResult(result ProviderFailureResult, cause error) *ProviderError {
	return &ProviderError{
		Family:  providerFailurePolicyForReason(result.Reason).Family,
		Type:    result.Reason,
		Message: result.Message,
		Cause:   cause,
	}
}

func NewProviderErrorWithSession(errorType interfaces.WorkFailureType, message string, cause error, session *interfaces.ProviderSessionMetadata) *ProviderError {
	err := NewProviderError(errorType, message, cause)
	err.ProviderSession = interfaces.CloneProviderSessionMetadata(session)
	return err
}

func newProviderErrorWithDiagnostics(errorType interfaces.WorkFailureType, message string, cause error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	err := NewProviderErrorWithSession(errorType, message, cause, session)
	err.Diagnostics = interfaces.CloneWorkDiagnostics(diagnostics)
	return err
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error: %s", e.Type)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ClassifyProviderFailure(err *ProviderError) interfaces.WorkFailureDecision {
	if err == nil {
		return interfaces.WorkFailureDecision{}
	}
	return providerFailurePolicyForReason(err.Type).Decision
}

// WorkFailureDecisionFromProviderError resolves retry behavior from a normalized
// provider error using the same FailureMetadata projection as WorkResult.
func WorkFailureDecisionFromProviderError(err *ProviderError) interfaces.WorkFailureDecision {
	return WorkFailureDecisionFromMetadata(WorkFailureMetadataFromError(err))
}

// WorkFailureDecisionFromMetadata resolves retry behavior from durable
// generalized failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func WorkFailureDecisionFromMetadata(metadata *interfaces.WorkFailureMetadata) interfaces.WorkFailureDecision {
	if metadata == nil {
		return interfaces.WorkFailureDecision{}
	}
	if metadata.Type != "" {
		return providerFailurePolicyForReason(metadata.Type).Decision
	}
	return providerFailureDecisionForFamily(metadata.Family)
}

type providerFailurePolicy struct {
	Family   interfaces.WorkFailureFamily
	Decision interfaces.WorkFailureDecision
}

func providerFailurePolicyForReason(reason interfaces.WorkFailureType) providerFailurePolicy {
	switch reason {
	case interfaces.WorkFailureTypeThrottled:
		return providerFailurePolicy{
			Family: interfaces.WorkFailureFamilyThrottle,
			Decision: interfaces.WorkFailureDecision{
				Retryable:             true,
				TriggersThrottlePause: true,
			},
		}
	case interfaces.WorkFailureTypeInternalServerError, interfaces.WorkFailureTypeTimeout:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyRetryable,
			Decision: interfaces.WorkFailureDecision{Retryable: true},
		}
	case interfaces.WorkFailureTypeAuthFailure,
		interfaces.WorkFailureTypePermanentBadRequest,
		interfaces.WorkFailureTypeUnknown,
		interfaces.WorkFailureTypeMisconfigured:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	default:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	}
}

func providerFailureDecisionForFamily(family interfaces.WorkFailureFamily) interfaces.WorkFailureDecision {
	switch family {
	case interfaces.WorkFailureFamilyRetryable:
		return interfaces.WorkFailureDecision{Retryable: true}
	case interfaces.WorkFailureFamilyThrottle:
		return interfaces.WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case interfaces.WorkFailureFamilyTerminal:
		return interfaces.WorkFailureDecision{Terminal: true}
	default:
		return interfaces.WorkFailureDecision{Terminal: true}
	}
}

func providerErrorFamilyForType(errorType interfaces.WorkFailureType) interfaces.WorkFailureFamily {
	return providerFailurePolicyForReason(errorType).Family
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the in-process failure contract carried on WorkResult.FailureMetadata.
func WorkFailureMetadataFromError(err *ProviderError) *interfaces.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	return &interfaces.WorkFailureMetadata{
		Family: providerFailurePolicyForReason(err.Type).Family,
		Type:   err.Type,
	}
}

// NormalizeProviderExecutionError projects raw execution failures that affect
// retry policy onto the shared provider failure contract before retry decisions
// are made.
func NormalizeProviderExecutionError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(interfaces.WorkFailureTypeTimeout, "execution timeout", err)
	}
	return nil
}
