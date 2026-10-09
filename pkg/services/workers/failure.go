package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerinferencefailure "github.com/portpowered/infinite-you/pkg/services/workers/internal/inferencefailure"
)

// InferenceFailureContext identifies the inference target for failure messages.
type InferenceFailureContext struct {
	ModelName  string
	WorkerName string
	Operation  string
}

// ClassifyInferenceFailure owns the conversion of model-readiness and Worker
// execution failures into one detached customer-safe result. Workers performs
// the classification; the classified value is Models-owned vocabulary
// (models.InferenceFailure) so outward adapters can consume it without
// depending on Workers.
func ClassifyInferenceFailure(err error, ctx InferenceFailureContext) (*models.InferenceFailure, bool) {
	var classified *models.InferenceFailure
	if errors.As(err, &classified) && classified != nil {
		return classified, true
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

func ClassifyInferenceWorkResultFailure(result WorkResult, ctx InferenceFailureContext) (*models.InferenceFailure, bool) {
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

func convertInferenceFailure(failure *workerinferencefailure.InferenceFailure) *models.InferenceFailure {
	if failure == nil {
		return nil
	}
	return &models.InferenceFailure{
		Class:      models.InferenceFailureClass(failure.Class),
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
	Family  WorkFailureFamily
	Type    WorkFailureType
	Message string
	// ProviderSession is retained for legacy Infer-shaped callers. Providers
	// owns the detached identity; new code uses Continuation at this boundary.
	ProviderSession                 *ProviderSessionMetadata
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
		if providerErr != nil && providerErr.Continuation == nil {
			providerErr.Continuation = (providerErr.ProviderSession).ContinuationRef()
		}
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
	// Structural matching keeps classification independent of inspection ownership.
	var sessionErr interface {
		error
		ProviderSessionFailureKind() string
	}
	if !errors.As(err, &sessionErr) {
		return nil
	}
	failureType := WorkFailureTypeUnknown
	classification := "storage"
	message := "provider session ingestion failed"
	switch {
	case hasProviderSessionFailureKind(err, "resource_limit"):
		classification = "resource_limit"
		message = "provider session inspection reached its configured limit"
	case hasProviderSessionFailureKind(err, "storage_unavailable"):
		failureType = WorkFailureTypeInternalServerError
		message = "provider session storage was unavailable"
	case hasProviderSessionFailureKind(err, "not_found"):
		failureType = WorkFailureTypePermanentBadRequest
		message = "provider session could not be found"
	case hasProviderSessionFailureKind(err, "canceled"),
		errors.Is(err, context.Canceled):
		classification = "canceled"
		message = "provider session inspection was canceled"
	case hasProviderSessionFailureKind(err, "invalid_identifier"),
		hasProviderSessionFailureKind(err, "unsupported_kind"),
		hasProviderSessionFailureKind(err, "unsupported_provider"):
		failureType = WorkFailureTypePermanentBadRequest
		message = "provider session request was invalid"
	}
	provider := ""
	sessionID := ""
	kind := "session_id"
	var identity interface {
		ProviderSessionFailureIdentity() (provider, kind, sessionID string)
	}
	if errors.As(err, &identity) {
		provider, kind, sessionID = identity.ProviderSessionFailureIdentity()
		provider = strings.TrimSpace(provider)
		sessionID = strings.TrimSpace(sessionID)
	}
	var continuation *ProviderContinuationRef
	if sessionID != "" {
		continuation = &ProviderContinuationRef{
			Provider:          provider,
			Kind:              kind,
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
		ProviderSession: (continuation).SessionMetadata(),
		Continuation:    continuation,
		Diagnostics:     diagnostics,
		Cause:           err,
	}
}

// Search the entire cause tree so joined failures keep the classifier's priority.
func hasProviderSessionFailureKind(err error, kind string) bool {
	var failure interface {
		error
		ProviderSessionFailureKind() string
	}
	if errors.As(err, &failure) && failure.ProviderSessionFailureKind() == kind {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if hasProviderSessionFailureKind(cause, kind) {
				return true
			}
		}
		return false
	}
	if cause := errors.Unwrap(err); cause != nil {
		return hasProviderSessionFailureKind(cause, kind)
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
	err.ProviderSession = (err.Continuation).SessionMetadata()
	return err
}

func WorkFailureMetadataFromProviderError(err *ProviderError) *WorkFailureMetadata {
	if err == nil {
		return nil
	}
	family := providerFailureFamily(err.Type)
	// Only native provider outages share capacity backpressure and its failure
	// budget exemption. Internal faults retain their short retry budget.
	if err.ProviderFailureKind == providers.ExecuteFailureKindDependency && err.Type == WorkFailureTypeInternalServerError {
		family = WorkFailureFamilyThrottle
	}
	return &WorkFailureMetadata{Family: family, Type: err.Type}
}

func WorkFailureDecisionFromProviderError(err *ProviderError) WorkFailureDecision {
	return FailureDecisionFromMetadata(WorkFailureMetadataFromProviderError(err))
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
