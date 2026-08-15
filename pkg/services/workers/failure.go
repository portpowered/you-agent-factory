package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerinferencefailure "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/inferencefailure"
)

// InferenceFailureClass is the stable customer-facing inference failure category.
type InferenceFailureClass string

const (
	InferenceFailureClassMissingModel         InferenceFailureClass = "missing_model"
	InferenceFailureClassLoadingModel         InferenceFailureClass = "loading_model"
	InferenceFailureClassUnsupportedOperation InferenceFailureClass = "unsupported_operation"
	InferenceFailureClassTimeout              InferenceFailureClass = "timeout"
	InferenceFailureClassRuntimeFailure       InferenceFailureClass = "runtime_failure"
)

// InferenceFailure is the detached, customer-safe outcome of Worker-owned
// inference failure classification.
type InferenceFailure struct {
	Class      InferenceFailureClass
	Message    string
	ModelName  string
	WorkerName string
	Operation  string
	Cause      error
}

func (e *InferenceFailure) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *InferenceFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// InferenceFailureContext identifies the inference target for failure messages.
type InferenceFailureContext struct {
	ModelName  string
	WorkerName string
	Operation  string
}

// AsInferenceFailure reports whether err contains a classified inference failure.
func AsInferenceFailure(err error) (*InferenceFailure, bool) {
	var failure *InferenceFailure
	if errors.As(err, &failure) && failure != nil {
		return failure, true
	}
	return nil, false
}

// ClassifyInferenceFailure owns the conversion of model-readiness and Worker
// execution failures into one detached customer-safe result.
func ClassifyInferenceFailure(err error, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	if failure, ok := AsInferenceFailure(err); ok {
		return failure, true
	}
	failure, ok := workerinferencefailure.ClassifyInferenceFailure(
		adaptClassificationError(err),
		workerinferencefailure.InferenceFailureContext{
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
		},
	)
	if !ok {
		return nil, false
	}
	return convertInferenceFailure(failure), true
}

func ClassifyInferenceWorkResultFailure(result WorkResult, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	failure, ok := workerinferencefailure.ClassifyInferenceWorkResultFailure(
		workerinferencefailure.WorkResult{
			Outcome:         workerinferencefailure.WorkResultOutcome(result.Outcome),
			Error:           result.Error,
			FailureMetadata: convertInferenceFailureMetadata(result.FailureMetadata),
		},
		workerinferencefailure.InferenceFailureContext{
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
		},
	)
	if !ok {
		return nil, false
	}
	return convertInferenceFailure(failure), true
}

func convertInferenceFailureMetadata(metadata *WorkFailureMetadata) *workerinferencefailure.WorkFailureMetadata {
	if metadata == nil {
		return nil
	}
	return &workerinferencefailure.WorkFailureMetadata{
		Type: workerinferencefailure.WorkFailureType(metadata.Type),
	}
}

func convertInferenceFailure(failure *workerinferencefailure.InferenceFailure) *InferenceFailure {
	if failure == nil {
		return nil
	}
	return &InferenceFailure{
		Class:      InferenceFailureClass(failure.Class),
		Message:    failure.Message,
		ModelName:  failure.ModelName,
		WorkerName: failure.WorkerName,
		Operation:  failure.Operation,
		Cause:      failure.Cause,
	}
}

func adaptClassificationError(err error) error {
	if err == nil {
		return nil
	}
	var targetErr *models.TargetError
	if errors.As(err, &targetErr) && targetErr != nil {
		return err
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		return &workerinferencefailure.ProviderError{
			Type:    workerinferencefailure.WorkFailureType(providerErr.Type),
			Message: providerErr.Message,
			Cause:   providerErr.Cause,
		}
	}
	return err
}

// ProviderError is the public normalized Worker provider failure.
type ProviderError struct {
	Family                          WorkFailureFamily
	Type                            WorkFailureType
	Message                         string
	Continuation                    *ProviderContinuationRef
	Diagnostics                     *WorkDiagnostics
	Cause                           error
	ProviderFailureKind             providers.ExecuteFailureKind
	ProviderContinuationFailureKind providers.ContinuationFailureKind
	ProviderContinuationOutcome     providers.ContinuationOutcome
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
	if providerErr := normalizeProviderSessionError(err); providerErr != nil {
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(WorkFailureTypeTimeout, "execution timeout", err)
	}
	return nil
}

func normalizeProviderSessionError(err error) *ProviderError {
	var lookupErr *providersessions.LookupError
	if !errors.As(err, &lookupErr) && !isProviderSessionFailure(err) {
		return nil
	}
	failureType := WorkFailureTypeUnknown
	classification := "storage"
	message := "provider session ingestion failed"
	switch {
	case errors.Is(err, providersessions.ErrResourceLimitExceeded):
		classification = "resource_limit"
		message = "provider session inspection reached its configured limit"
	case errors.Is(err, providersessions.ErrSessionStorageUnavailable):
		failureType = WorkFailureTypeInternalServerError
		message = "provider session storage was unavailable"
	case errors.Is(err, providersessions.ErrSessionNotFound):
		failureType = WorkFailureTypePermanentBadRequest
		message = "provider session could not be found"
	case errors.Is(err, providersessions.ErrOperationCanceled),
		errors.Is(err, context.Canceled):
		classification = "canceled"
		message = "provider session inspection was canceled"
	case errors.Is(err, providersessions.ErrInvalidIdentifier),
		errors.Is(err, providersessions.ErrUnsupportedKind),
		errors.Is(err, providersessions.ErrUnsupportedProvider):
		failureType = WorkFailureTypePermanentBadRequest
		message = "provider session request was invalid"
	}
	provider := ""
	sessionID := ""
	if lookupErr != nil {
		provider = strings.TrimSpace(string(lookupErr.Provider))
		sessionID = strings.TrimSpace(lookupErr.SessionID)
	}
	var continuation *ProviderContinuationRef
	if sessionID != "" {
		continuation = &ProviderContinuationRef{
			Provider:          provider,
			Kind:              providersessions.SessionIDKind,
			ProviderSessionID: sessionID,
			ExternalRef:       sessionID,
		}
	}
	diagnostics := &WorkDiagnostics{
		Provider: &ProviderDiagnostic{
			Provider: provider,
			ResponseMetadata: map[string]string{
				ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
				ProviderResponseMetadataFailureClassification: classification,
			},
		},
	}
	if continuation != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = continuation.Provider
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = continuation.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = continuation.ProviderSessionID
	}
	return &ProviderError{
		Family:          providerFailureFamily(failureType),
		Type:            failureType,
		Message:         message,
		Continuation: continuation,
		Diagnostics:     diagnostics,
		Cause:           err,
	}
}

func isProviderSessionFailure(err error) bool {
	for _, candidate := range []error{
		providersessions.ErrAmbiguousSessionFile,
		providersessions.ErrInvalidIdentifier,
		providersessions.ErrOperationCanceled,
		providersessions.ErrResourceLimitExceeded,
		providersessions.ErrSessionNotFound,
		providersessions.ErrSessionOutsideRoot,
		providersessions.ErrSessionSourceNotRegularFile,
		providersessions.ErrSessionStorageUnavailable,
		providersessions.ErrUnsupportedKind,
		providersessions.ErrUnsupportedProvider,
	} {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}

func NewProviderErrorWithSession(
	failureType WorkFailureType,
	message string,
	cause error,
	continuation *ProviderContinuationRef,
) *ProviderError {
	err := NewProviderError(failureType, message, cause)
	err.Continuation = cloneContinuation(continuation)
	return err
}

func WorkFailureMetadataFromProviderError(err *ProviderError) *WorkFailureMetadata {
	if err == nil {
		return nil
	}
	return &WorkFailureMetadata{Family: providerFailureFamily(err.Type), Type: err.Type}
}

func WorkFailureDecisionFromProviderError(err *ProviderError) WorkFailureDecision {
	return FailureDecisionFromMetadata(WorkFailureMetadataFromProviderError(err))
}

func WorkFailureDecisionFromMetadata(metadata *WorkFailureMetadata) WorkFailureDecision {
	return FailureDecisionFromMetadata(metadata)
}

func ContainsStopToken(output, stopToken string) bool {
	if stopToken == "" {
		return false
	}
	if stopToken != "<COMPLETE>" {
		return strings.Contains(output, stopToken)
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			return line == stopToken
		}
	}
	return false
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
