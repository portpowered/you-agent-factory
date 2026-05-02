package workers

import (
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
	err.ProviderSession = cloneProviderSession(session)
	return err
}

func newProviderErrorWithDiagnostics(errorType interfaces.ProviderErrorType, message string, cause error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	err := NewProviderErrorWithSession(errorType, message, cause, session)
	err.Diagnostics = cloneWorkDiagnostics(diagnostics)
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

func ClassifyProviderFailure(err *ProviderError) interfaces.ProviderFailureDecision {
	if err == nil {
		return interfaces.ProviderFailureDecision{}
	}
	return providerFailureDecisionForFamily(err.Family)
}

// ProviderFailureDecisionFromMetadata resolves retry behavior from the durable
// normalized provider-failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func ProviderFailureDecisionFromMetadata(metadata *interfaces.ProviderFailureMetadata) interfaces.ProviderFailureDecision {
	if metadata == nil {
		return interfaces.ProviderFailureDecision{}
	}
	if metadata.Type != "" {
		return providerFailureDecisionForFamily(providerErrorFamilyForType(metadata.Type))
	}
	return providerFailureDecisionForFamily(metadata.Family)
}

func providerFailureDecisionForFamily(family interfaces.ProviderErrorFamily) interfaces.ProviderFailureDecision {
	switch family {
	case interfaces.ProviderErrorFamilyRetryable:
		return interfaces.ProviderFailureDecision{Retryable: true}
	case interfaces.ProviderErrorFamilyThrottle:
		return interfaces.ProviderFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case interfaces.ProviderErrorFamilyTerminal:
		return interfaces.ProviderFailureDecision{Terminal: true}
	default:
		return interfaces.ProviderFailureDecision{Terminal: true}
	}
}

func providerErrorFamilyForType(errorType interfaces.ProviderErrorType) interfaces.ProviderErrorFamily {
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

func ProviderFailureMetadataFromError(err *ProviderError) *interfaces.ProviderFailureMetadata {
	if err == nil {
		return nil
	}
	return &interfaces.ProviderFailureMetadata{
		Family: err.Family,
		Type:   err.Type,
	}
}

func cloneProviderSession(session *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}
