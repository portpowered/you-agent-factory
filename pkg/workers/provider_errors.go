package workers

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
	Family          interfaces.ProviderErrorFamily
	Type            interfaces.ProviderErrorType
	Message         string
	ProviderSession *interfaces.ProviderSessionMetadata
	Diagnostics     *interfaces.WorkDiagnostics
	Cause           error
}

func NewProviderError(errorType interfaces.ProviderErrorType, message string, cause error) *ProviderError {
	return &ProviderError{
		Family:  providerErrorFamilyForType(errorType),
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}

func NewProviderErrorWithSession(errorType interfaces.ProviderErrorType, message string, cause error, session *interfaces.ProviderSessionMetadata) *ProviderError {
	err := NewProviderError(errorType, message, cause)
	err.ProviderSession = interfaces.CloneProviderSessionMetadata(session)
	return err
}

func newProviderErrorWithDiagnostics(errorType interfaces.ProviderErrorType, message string, cause error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
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
	return providerFailureDecisionForFamily(err.Family)
}

// ProviderFailureDecisionFromMetadata resolves retry behavior from the durable
// normalized provider-failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func ProviderFailureDecisionFromMetadata(metadata *interfaces.ProviderFailureMetadata) interfaces.WorkFailureDecision {
	return WorkFailureDecisionFromMetadata(metadata)
}

// WorkFailureDecisionFromMetadata resolves retry behavior from durable
// generalized failure metadata carried across runtime boundaries.
func WorkFailureDecisionFromMetadata(metadata *interfaces.WorkFailureMetadata) interfaces.WorkFailureDecision {
	if metadata == nil {
		return interfaces.WorkFailureDecision{}
	}
	if metadata.Type != "" {
		return providerFailureDecisionForFamily(providerErrorFamilyForType(metadata.Type))
	}
	return providerFailureDecisionForFamily(metadata.Family)
}

func providerFailureDecisionForFamily(family interfaces.WorkFailureFamily) interfaces.WorkFailureDecision {
	switch family {
	case interfaces.ProviderErrorFamilyRetryable:
		return interfaces.WorkFailureDecision{Retryable: true}
	case interfaces.ProviderErrorFamilyThrottle:
		return interfaces.WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case interfaces.ProviderErrorFamilyTerminal:
		return interfaces.WorkFailureDecision{Terminal: true}
	default:
		return interfaces.WorkFailureDecision{Terminal: true}
	}
}

func providerErrorFamilyForType(errorType interfaces.WorkFailureType) interfaces.WorkFailureFamily {
	switch errorType {
	case interfaces.ProviderErrorTypeThrottled:
		return interfaces.ProviderErrorFamilyThrottle
	case interfaces.ProviderErrorTypeInternalServerError, interfaces.ProviderErrorTypeTimeout:
		return interfaces.ProviderErrorFamilyRetryable
	case interfaces.ProviderErrorTypeAuthFailure, interfaces.ProviderErrorTypePermanentBadRequest, interfaces.ProviderErrorTypeUnknown, interfaces.ProviderErrorTypeMisconfigured:
		return interfaces.ProviderErrorFamilyTerminal
	default:
		return interfaces.ProviderErrorFamilyTerminal
	}
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the generalized runtime failure contract.
func WorkFailureMetadataFromError(err *ProviderError) *interfaces.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	return &interfaces.WorkFailureMetadata{
		Family: err.Family,
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
		return NewProviderError(interfaces.ProviderErrorTypeTimeout, "execution timeout", err)
	}
	return nil
}
