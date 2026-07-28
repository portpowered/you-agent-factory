package workers

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/models"
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
