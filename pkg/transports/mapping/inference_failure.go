package apisurface

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
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

// InferenceFailure carries actionable inference failure context shared by direct
// model invocation and factory-session inference execution surfaces.
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

// AsInferenceFailure reports whether err is a classified inference failure.
func AsInferenceFailure(err error) (*InferenceFailure, bool) {
	var failure *InferenceFailure
	if errors.As(err, &failure) && failure != nil {
		return failure, true
	}
	return nil, false
}

// ClassifyInferenceFailure maps one inference boundary error into an actionable
// customer-facing failure without exposing raw subprocess output.
func ClassifyInferenceFailure(err error, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	if err == nil {
		return nil, false
	}
	if failure, ok := AsInferenceFailure(err); ok {
		return failure, true
	}

	var readinessErr *ManagedRuntimeInvocationError
	if errors.As(err, &readinessErr) {
		return classifyManagedRuntimeFailure(readinessErr, ctx), true
	}
	switch {
	case errors.Is(err, ErrManagedRuntimeMissing), IsManagedRuntimeMissing(err), errors.Is(err, ErrModelNotAvailable):
		return missingModelFailure(ctx, err), true
	case errors.Is(err, ErrManagedRuntimeLoading):
		return loadingModelFailure(ctx, err), true
	case errors.Is(err, ErrManagedRuntimeFailed), errors.Is(err, ErrManagedRuntimeUnsupported):
		return runtimeReadinessFailure(ctx, err), true
	case errors.Is(err, ErrModelInvocationUnsupportedOperation):
		return unsupportedOperationFailure(err, ctx), true
	}

	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) {
		return classifyProviderFailure(providerErr, ctx), true
	}
	if normalized := workerprovider.NormalizeProviderExecutionError(err); normalized != nil {
		return classifyProviderFailure(normalized, ctx), true
	}
	return classifyRawExecutionFailure(err, ctx)
}

// ClassifyInferenceWorkResultFailure maps one failed inference workstation
// result onto the shared inference failure contract.
func ClassifyInferenceWorkResultFailure(result workerexecution.WorkResult, ctx InferenceFailureContext) (*InferenceFailure, bool) {
	if result.Outcome != workerexecution.OutcomeFailed {
		return nil, false
	}
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		providerErr := workerprovider.NewProviderError(
			result.FailureMetadata.Type,
			boundedInferenceMessage(result.Error),
			nil,
		)
		return classifyProviderFailure(providerErr, ctx), true
	}
	errText := strings.TrimSpace(result.Error)
	if errText == "" {
		return runtimeFailure(ctx, "inference execution failed", nil), true
	}
	return classifyRawExecutionFailure(errors.New(errText), ctx)
}

// InferenceFailureHTTPStatus maps one inference failure to an HTTP status code.
func InferenceFailureHTTPStatus(failure *InferenceFailure) int {
	switch failure.Class {
	case InferenceFailureClassMissingModel:
		return http.StatusNotFound
	case InferenceFailureClassLoadingModel:
		return http.StatusConflict
	case InferenceFailureClassUnsupportedOperation:
		return http.StatusBadRequest
	case InferenceFailureClassTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// InferenceFailureErrorCode maps one inference failure to a stable API error code.
func InferenceFailureErrorCode(failure *InferenceFailure) string {
	switch failure.Class {
	case InferenceFailureClassMissingModel:
		return "MODEL_NOT_AVAILABLE"
	case InferenceFailureClassLoadingModel:
		return "MODEL_RUNTIME_LOADING"
	case InferenceFailureClassUnsupportedOperation:
		return "BAD_REQUEST"
	case InferenceFailureClassTimeout:
		return "MODEL_INFERENCE_TIMEOUT"
	default:
		return "MODEL_INFERENCE_RUNTIME_FAILURE"
	}
}

func classifyManagedRuntimeFailure(readinessErr *ManagedRuntimeInvocationError, ctx InferenceFailureContext) *InferenceFailure {
	if readinessErr == nil {
		return runtimeFailure(ctx, "resolve managed runtime readiness before invoking", nil)
	}
	message := strings.TrimSpace(readinessErr.Error())
	switch {
	case errors.Is(readinessErr, ErrManagedRuntimeMissing):
		return missingModelFailure(ctx, readinessErr)
	case errors.Is(readinessErr, ErrManagedRuntimeLoading):
		return loadingModelFailure(ctx, readinessErr)
	default:
		return &InferenceFailure{
			Class:      InferenceFailureClassRuntimeFailure,
			Message:    message,
			ModelName:  firstNonEmpty(ctx.ModelName, readinessErr.Identity),
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
			Cause:      readinessErr,
		}
	}
}

func missingModelFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	return &InferenceFailure{
		Class: InferenceFailureClassMissingModel,
		Message: fmt.Sprintf(
			"model %q is not available: pull or install the managed runtime before invoking",
			modelName,
		),
		ModelName:  modelName,
		WorkerName: ctx.WorkerName,
		Operation:  ctx.Operation,
		Cause:      cause,
	}
}

func loadingModelFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	return &InferenceFailure{
		Class: InferenceFailureClassLoadingModel,
		Message: fmt.Sprintf(
			"model %q is still loading: wait for the managed runtime to finish loading and retry the invocation",
			modelName,
		),
		ModelName:  modelName,
		WorkerName: ctx.WorkerName,
		Operation:  ctx.Operation,
		Cause:      cause,
	}
}

func runtimeReadinessFailure(ctx InferenceFailureContext, cause error) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	message := "resolve the managed runtime failure before invoking"
	if readinessErr, ok := cause.(*ManagedRuntimeInvocationError); ok && strings.TrimSpace(readinessErr.Error()) != "" {
		message = strings.TrimSpace(readinessErr.Error())
	}
	return &InferenceFailure{
		Class:      InferenceFailureClassRuntimeFailure,
		Message:    message,
		ModelName:  modelName,
		WorkerName: ctx.WorkerName,
		Operation:  ctx.Operation,
		Cause:      cause,
	}
}

func unsupportedOperationFailure(err error, ctx InferenceFailureContext) *InferenceFailure {
	modelName := firstNonEmpty(ctx.ModelName, "model")
	operation := firstNonEmpty(ctx.Operation, "operation")
	message := fmt.Sprintf(
		"model %q does not support operation %q",
		modelName,
		operation,
	)
	if workerName := strings.TrimSpace(ctx.WorkerName); workerName != "" {
		message = fmt.Sprintf(
			"worker %q for model %q does not support operation %q",
			workerName,
			modelName,
			operation,
		)
	}
	if trimmed := strings.TrimSpace(err.Error()); trimmed != "" && strings.Contains(trimmed, "does not support operation") {
		message = boundedInferenceMessage(trimmed)
	} else if trimmed := strings.TrimSpace(err.Error()); trimmed != "" && !errors.Is(err, ErrModelInvocationUnsupportedOperation) {
		message = boundedInferenceMessage(trimmed)
	}
	return &InferenceFailure{
		Class:      InferenceFailureClassUnsupportedOperation,
		Message:    message,
		ModelName:  modelName,
		WorkerName: ctx.WorkerName,
		Operation:  operation,
		Cause:      err,
	}
}

func classifyProviderFailure(providerErr *workerprovider.ProviderError, ctx InferenceFailureContext) *InferenceFailure {
	if providerErr == nil {
		return runtimeFailure(ctx, "inference execution failed", nil)
	}
	message := boundedInferenceMessage(providerErr.Message)
	if isRawSubprocessFailureMessage(message) {
		message = ""
	}
	if providerErr.Type == workerexecution.WorkFailureTypeTimeout {
		return &InferenceFailure{
			Class: InferenceFailureClassTimeout,
			Message: fmt.Sprintf(
				"inference timed out for model %q operation %q: wait and retry the request",
				firstNonEmpty(ctx.ModelName, "model"),
				firstNonEmpty(ctx.Operation, "operation"),
			),
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
			Cause:      providerErr,
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
	if strings.Contains(strings.ToLower(message), "timeout") || strings.Contains(strings.ToLower(message), "deadline exceeded") {
		return &InferenceFailure{
			Class: InferenceFailureClassTimeout,
			Message: fmt.Sprintf(
				"inference timed out for model %q operation %q: wait and retry the request",
				firstNonEmpty(ctx.ModelName, "model"),
				firstNonEmpty(ctx.Operation, "operation"),
			),
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
			Cause:      err,
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
		Class:      InferenceFailureClassRuntimeFailure,
		Message:    boundedInferenceMessage(message),
		ModelName:  modelName,
		WorkerName: workerName,
		Operation:  operation,
		Cause:      cause,
	}
}

func boundedInferenceMessage(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if message == "" {
		return ""
	}
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
	for _, marker := range []string{
		"exited with code",
		"run local ",
		"invoke supervised ",
		"provider execution failed:",
		"subprocess",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
