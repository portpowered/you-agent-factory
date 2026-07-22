package workers

import (
	"context"
	"errors"
	"fmt"
)

// ProviderError is the public normalized Worker provider failure.
type ProviderError struct {
	Family          WorkFailureFamily
	Type            WorkFailureType
	Message         string
	ProviderSession *ProviderSessionMetadata
	Diagnostics     *WorkDiagnostics
	Cause           error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("provider error: %s", e.Type)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewProviderError(
	failureType WorkFailureType,
	message string,
	cause error,
) *ProviderError {
	return &ProviderError{
		Family:  providerFailureFamily(failureType),
		Type:    failureType,
		Message: message,
		Cause:   cause,
	}
}

func NormalizeProviderExecutionError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(WorkFailureTypeTimeout, "execution timeout", err)
	}
	return nil
}

func providerFailureFamily(failureType WorkFailureType) WorkFailureFamily {
	switch failureType {
	case WorkFailureTypeThrottled:
		return WorkFailureFamilyThrottle
	case WorkFailureTypeInternalServerError, WorkFailureTypeTimeout:
		return WorkFailureFamilyRetryable
	default:
		return WorkFailureFamilyTerminal
	}
}
