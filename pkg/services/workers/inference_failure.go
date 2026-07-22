package workers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
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

const inferenceFailureMessageLimit = 160

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
	if err == nil {
		return nil, false
	}
	if failure, ok := AsInferenceFailure(err); ok {
		return failure, true
	}
	var target *models.TargetError
	if errors.As(err, &target) && target != nil {
		ctx.ModelName = firstNonEmpty(target.ModelName, ctx.ModelName)
		ctx.WorkerName = firstNonEmpty(target.WorkerName, ctx.WorkerName)
		ctx.Operation = firstNonEmpty(target.Operation, ctx.Operation)
	}
	var readinessErr *models.InvocationError
	if errors.As(err, &readinessErr) {
		return classifyManagedRuntimeFailure(readinessErr, ctx), true
	}
	switch {
	case errors.Is(err, models.ErrMissing), errors.Is(err, models.ErrNotAvailable):
		return missingModelFailure(ctx, err), true
	case errors.Is(err, models.ErrLoading):
		return loadingModelFailure(ctx, err), true
	case errors.Is(err, models.ErrFailed), errors.Is(err, models.ErrUnsupported):
		return runtimeReadinessFailure(ctx, err), true
	case errors.Is(err, models.ErrUnsupportedOperation):
		return unsupportedOperationFailure(err, ctx), true
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return classifyProviderFailure(providerErr, ctx), true
	}
	if normalized := NormalizeProviderExecutionError(err); normalized != nil {
		return classifyProviderFailure(normalized, ctx), true
	}
	return classifyRawExecutionFailure(err, ctx)
}

// ClassifyInferenceWorkResultFailure owns failed Work-result classification.
func ClassifyInferenceWorkResultFailure(result WorkResult, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	if result.Outcome != OutcomeFailed {
		return nil, false
	}
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		return classifyProviderFailure(NewProviderError(
			result.FailureMetadata.Type,
			boundedInferenceMessage(result.Error),
			nil,
		), ctx), true
	}
	errText := strings.TrimSpace(result.Error)
	if errText == "" {
		return runtimeFailure(ctx, "inference execution failed", nil), true
	}
	return classifyRawExecutionFailure(errors.New(errText), ctx)
}

func classifyManagedRuntimeFailure(readinessErr *models.InvocationError, ctx InferenceFailureContext) *InferenceFailure {
	if readinessErr == nil {
		return runtimeFailure(ctx, "resolve managed runtime readiness before invoking", nil)
	}
	switch {
	case errors.Is(readinessErr, models.ErrMissing):
		return missingModelFailure(ctx, readinessErr)
	case errors.Is(readinessErr, models.ErrLoading):
		return loadingModelFailure(ctx, readinessErr)
	default:
		return &InferenceFailure{
			Class: InferenceFailureClassRuntimeFailure, Message: strings.TrimSpace(readinessErr.Error()),
			ModelName: firstNonEmpty(ctx.ModelName, readinessErr.Identity), WorkerName: ctx.WorkerName,
			Operation: ctx.Operation, Cause: readinessErr,
		}
	}
}

func missingModelFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	return &InferenceFailure{
		Class:     InferenceFailureClassMissingModel,
		Message:   fmt.Sprintf("model %q is not available: pull or install the managed runtime before invoking", modelName),
		ModelName: modelName, WorkerName: ctx.WorkerName, Operation: ctx.Operation, Cause: cause,
	}
}

func loadingModelFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	return &InferenceFailure{
		Class:     InferenceFailureClassLoadingModel,
		Message:   fmt.Sprintf("model %q is still loading: wait for the managed runtime to finish loading and retry the invocation", modelName),
		ModelName: modelName, WorkerName: ctx.WorkerName, Operation: ctx.Operation, Cause: cause,
	}
}

func runtimeReadinessFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	message := "resolve the managed runtime failure before invoking"
	var readinessErr *models.InvocationError
	if errors.As(cause, &readinessErr) && strings.TrimSpace(readinessErr.Error()) != "" {
		message = strings.TrimSpace(readinessErr.Error())
	}
	return &InferenceFailure{
		Class: InferenceFailureClassRuntimeFailure, Message: message,
		ModelName: firstNonEmpty(ctx.ModelName, "model"), WorkerName: ctx.WorkerName,
		Operation: ctx.Operation, Cause: cause,
	}
}

func unsupportedOperationFailure(err error, ctx InferenceFailureContext) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	operation := firstNonEmpty(ctx.Operation, "operation")
	message := fmt.Sprintf("model %q does not support operation %q", modelName, operation)
	if workerName := strings.TrimSpace(ctx.WorkerName); workerName != "" {
		message = fmt.Sprintf("worker %q for model %q does not support operation %q", workerName, modelName, operation)
	}
	if trimmed := strings.TrimSpace(err.Error()); trimmed != "" && strings.Contains(trimmed, "does not support operation") {
		message = boundedInferenceMessage(trimmed)
	} else if trimmed != "" && !errors.Is(err, models.ErrUnsupportedOperation) {
		message = boundedInferenceMessage(trimmed)
	}
	return &InferenceFailure{
		Class: InferenceFailureClassUnsupportedOperation, Message: message,
		ModelName: modelName, WorkerName: ctx.WorkerName, Operation: operation, Cause: err,
	}
}

func classifyProviderFailure(providerErr *ProviderError, ctx InferenceFailureContext) *InferenceFailure {
	if providerErr == nil {
		return runtimeFailure(ctx, "inference execution failed", nil)
	}
	message := boundedInferenceMessage(providerErr.Message)
	if isRawSubprocessFailureMessage(message) {
		message = ""
	}
	if providerErr.Type == WorkFailureTypeTimeout {
		return &InferenceFailure{
			Class:     InferenceFailureClassTimeout,
			Message:   fmt.Sprintf("inference timed out for model %q operation %q: wait and retry the request", firstNonEmpty(ctx.ModelName, "model"), firstNonEmpty(ctx.Operation, "operation")),
			ModelName: ctx.ModelName, WorkerName: ctx.WorkerName, Operation: ctx.Operation, Cause: providerErr,
		}
	}
	if message == "" {
		message = "inference execution failed"
	}
	return runtimeFailure(ctx, message, providerErr)
}

func classifyRawExecutionFailure(err error, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	if err == nil {
		return nil, false
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return runtimeFailure(ctx, "inference execution failed", err), true
	}
	if strings.HasPrefix(message, "provider execution failed:") {
		message = strings.TrimSpace(strings.TrimPrefix(message, "provider execution failed:"))
	}
	if isRawSubprocessFailureMessage(message) {
		return runtimeFailure(ctx, "inference runtime failed", err), true
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		return &InferenceFailure{
			Class:     InferenceFailureClassTimeout,
			Message:   fmt.Sprintf("inference timed out for model %q operation %q: wait and retry the request", firstNonEmpty(ctx.ModelName, "model"), firstNonEmpty(ctx.Operation, "operation")),
			ModelName: ctx.ModelName, WorkerName: ctx.WorkerName, Operation: ctx.Operation, Cause: err,
		}, true
	}
	return runtimeFailure(ctx, boundedInferenceMessage(message), err), true
}

func runtimeFailure(ctx InferenceFailureContext, detail string, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	workerName := strings.TrimSpace(ctx.WorkerName)
	operation := firstNonEmpty(ctx.Operation, "operation")
	message := fmt.Sprintf("inference failed for model %q operation %q", modelName, operation)
	if workerName != "" {
		message = fmt.Sprintf("inference failed for worker %q model %q operation %q", workerName, modelName, operation)
	}
	if detail := strings.TrimSpace(detail); detail != "" && !isRawSubprocessFailureMessage(detail) {
		message = fmt.Sprintf("%s: %s", message, detail)
	}
	return &InferenceFailure{
		Class: InferenceFailureClassRuntimeFailure, Message: boundedInferenceMessage(message),
		ModelName: modelName, WorkerName: workerName, Operation: operation, Cause: cause,
	}
}

func boundedInferenceMessage(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) <= inferenceFailureMessageLimit {
		return message
	}
	return message[:inferenceFailureMessageLimit] + "..."
}

func isRawSubprocessFailureMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if len(message) > inferenceFailureMessageLimit {
		return true
	}
	for _, marker := range []string{"exited with code", "run local ", "invoke supervised ", "provider execution failed:", "subprocess"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
