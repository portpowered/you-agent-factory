package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	modelsRootModelNotFoundCode    = "NOT_FOUND"
	modelsRootModelUnavailableCode = "MODEL_NOT_AVAILABLE"
	modelsRootBadRequestCode       = "BAD_REQUEST"
	modelsRootCacheNotFoundCode    = "MODEL_CACHE_NOT_FOUND"
	modelsRootCacheInUseCode       = "MODEL_CACHE_IN_USE"
	modelsRootCacheUnsafeCode      = modelsRootBadRequestCode
	modelsRootPullFailedCode       = "CLI_MODEL_PULL_FAILED"
	modelsRootDefaultErrorText     = "models command failed"
	modelsRootMissingCachePrefix   = "model cache is not installed; run you models pull"
	modelsMalformedResponseCode    = "MODEL_BACKEND_FAILURE"
)

const modelsFactoryLayoutNotFoundCode = "CURRENT_FACTORY_NOT_FOUND"

// modelsRootError preserves a Models CLI sentinel and the originating Models
// error while exposing the safe diagnostic fields expected by the central CLI
// renderer. Its message is authored at this adapter boundary; the original
// cause remains available to errors.Is/errors.As and --debug callers.
type modelsRootError struct {
	code     string
	family   factoryapi.ErrorFamily
	message  string
	sentinel error
	cause    error
}

func (err *modelsRootError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", err.CLIErrorCode(), err.CLIErrorMessage())
}

func (err *modelsRootError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *modelsRootError) Is(target error) bool {
	if err == nil || err.sentinel == nil {
		return false
	}
	return errors.Is(err.sentinel, target)
}

func (err *modelsRootError) CLIErrorCode() string {
	if err == nil || strings.TrimSpace(err.code) == "" {
		return modelsRootCacheNotFoundCode
	}
	return strings.TrimSpace(err.code)
}

func (err *modelsRootError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil || err.family == "" {
		return factoryapi.ErrorFamilyInternalServerError
	}
	return err.family
}

func (err *modelsRootError) CLIErrorMessage() string {
	if err == nil || strings.TrimSpace(err.message) == "" {
		return modelsRootDefaultErrorText
	}
	return strings.TrimSpace(err.message)
}

func newModelsRootError(
	code string,
	family factoryapi.ErrorFamily,
	message string,
	sentinel error,
	cause error,
) error {
	return &modelsRootError{
		code: code, family: family, message: message, sentinel: sentinel, cause: cause,
	}
}

// modelsFactoryLayoutNotFoundError preserves the searched Factory root while
// exposing the not-found family expected by the process CLI boundary. The
// resolver's cause remains available to callers that need errors.Is without
// making the shared CLI renderer inspect private Factory Session errors.
type modelsFactoryLayoutNotFoundError struct {
	cause error
}

func (err *modelsFactoryLayoutNotFoundError) Error() string {
	if err == nil || err.cause == nil {
		return "Factory layout was not found"
	}
	return err.cause.Error()
}

func (err *modelsFactoryLayoutNotFoundError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *modelsFactoryLayoutNotFoundError) CLIErrorCode() string {
	return modelsFactoryLayoutNotFoundCode
}

func (err *modelsFactoryLayoutNotFoundError) CLIErrorFamily() factoryapi.ErrorFamily {
	return factoryapi.ErrorFamilyNotFound
}

func (err *modelsFactoryLayoutNotFoundError) CLIErrorMessage() string {
	return err.Error()
}

// mapModelsInvocationError preserves the Models invocation taxonomy at the
// local CLI boundary. Generic invocation validation already carries a safe
// public message; only the CLI diagnostic fields are missing when the error
// crosses this adapter. The underlying typed failure remains in the cause so
// callers and --debug diagnostics retain its identity.
func mapModelsInvocationError(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	var failure *modelinference.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		return nil, false
	}
	code, family := modelsInvocationDiagnostic(failure.Class)
	return newModelsRootError(code, family, failure.Error(), nil, err), true
}

func modelsInvocationDiagnostic(class modelinference.InvocationFailureClass) (string, factoryapi.ErrorFamily) {
	switch class {
	case modelinference.InvocationFailureClassInvalidModelReference,
		modelinference.InvocationFailureClassRevisionResolution:
		return modelsRootModelUnavailableCode, factoryapi.ErrorFamilyNotFound
	case modelinference.InvocationFailureClassInvalidOperation,
		modelinference.InvocationFailureClassInvalidSlot,
		modelinference.InvocationFailureClassSlotArity,
		modelinference.InvocationFailureClassInvalidParameter,
		modelinference.InvocationFailureClassMediaCapability,
		modelinference.InvocationFailureClassArtifact:
		return modelsRootBadRequestCode, factoryapi.ErrorFamilyBadRequest
	case modelinference.InvocationFailureClassOfflineCache:
		return "MODEL_OFFLINE_CACHE_UNAVAILABLE", factoryapi.ErrorFamilyConflict
	case modelinference.InvocationFailureClassBackendReadiness:
		return "MODEL_BACKEND_NOT_READY", factoryapi.ErrorFamilyInternalServerError
	case modelinference.InvocationFailureClassBackendProtocol,
		modelinference.InvocationFailureClassMalformedResponse:
		return "MODEL_BACKEND_FAILURE", factoryapi.ErrorFamilyInternalServerError
	case modelinference.InvocationFailureClassCancellation,
		modelinference.InvocationFailureClassTimeout:
		return "MODEL_INFERENCE_TIMEOUT", factoryapi.ErrorFamilyInternalServerError
	case modelinference.InvocationFailureClassConfiguration:
		return "MODEL_CONFIGURATION_FAILURE", factoryapi.ErrorFamilyInternalServerError
	case modelinference.InvocationFailureClassAssetPreparation:
		return "MODEL_ASSET_PREPARATION_FAILED", factoryapi.ErrorFamilyInternalServerError
	default:
		return "MODEL_INFERENCE_RUNTIME_FAILURE", factoryapi.ErrorFamilyInternalServerError
	}
}

func assetPreflightInvocationError(modelName, operation string, err error) error {
	if err == nil {
		return err
	}
	class := modelinference.InvocationFailureClassAssetPreparation
	message := "model asset preparation failed"
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, modelinference.ErrAssetCancelled):
		class = modelinference.InvocationFailureClassCancellation
		message = "model asset preparation was cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		class = modelinference.InvocationFailureClassTimeout
		message = "model asset preparation timed out"
	case errors.Is(err, modelinference.ErrAssetBackendNotReady):
		class = modelinference.InvocationFailureClassBackendReadiness
		message = "managed model backend is unavailable"
	case errors.Is(err, modelinference.ErrAssetOffline):
		class = modelinference.InvocationFailureClassOfflineCache
		message = "required model assets are unavailable offline"
	case errors.Is(err, modelinference.ErrModelRevisionUnresolved):
		class = modelinference.InvocationFailureClassRevisionResolution
		message = "model source revision could not be resolved to an immutable commit"
	}
	return &modelinference.InvocationFailure{
		Class: class, Message: message,
		Model:     modelinference.ModelReference{NameOrURI: strings.TrimSpace(modelName)},
		Operation: strings.TrimSpace(operation), Cause: err,
	}
}

func mapModelsClientError(err error) error {
	if mapped, ok := mapModelsInvocationError(err); ok {
		return mapped
	}
	return mapModelsRootError(err)
}

func malformedModelsResponseError(cause error) error {
	return mapModelsClientError(&modelinference.InvocationFailure{
		Class:   modelinference.InvocationFailureClassMalformedResponse,
		Message: "malformed models response",
		Cause:   cause,
	})
}

func mapModelsRootError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound):
		return &modelsFactoryLayoutNotFoundError{cause: err}
	case errors.Is(err, modelinference.ErrModelCacheNotFound):
		return newModelsRootError(
			modelsRootCacheNotFoundCode,
			factoryapi.ErrorFamilyNotFound,
			modelCacheNotFoundMessage(err),
			ErrModelCacheNotFound,
			err,
		)
	case errors.Is(err, modelinference.ErrModelCacheInUse):
		return newModelsRootError(
			modelsRootCacheInUseCode,
			factoryapi.ErrorFamilyConflict,
			strings.TrimSpace(err.Error()),
			ErrModelCacheInUse,
			err,
		)
	case errors.Is(err, modelinference.ErrModelCacheUnsafe):
		return newModelsRootError(
			modelsRootCacheUnsafeCode,
			factoryapi.ErrorFamilyBadRequest,
			strings.TrimSpace(err.Error()),
			ErrModelCacheUnsafe,
			err,
		)
	case errors.Is(err, modelinference.ErrNotFound):
		return newModelsRootError(
			modelsRootModelNotFoundCode,
			factoryapi.ErrorFamilyNotFound,
			modelNotFoundMessage(err),
			ErrModelNotFound,
			err,
		)
	case errors.Is(err, modelinference.ErrMissing),
		errors.Is(err, modelinference.ErrLoading),
		errors.Is(err, modelinference.ErrFailed),
		errors.Is(err, modelinference.ErrUnsupported),
		errors.Is(err, modelinference.ErrNotAvailable):
		return err
	case errors.Is(err, modelinference.ErrUnsupportedOperation),
		errors.Is(err, modelinference.ErrUnsupportedResponseMode),
		errors.Is(err, modelinference.ErrUnsupportedModelOperation):
		return err
	default:
		var pullErr *modelinference.PullError
		if errors.As(err, &pullErr) && pullErr != nil {
			diagnostics := pullsupport.MergePullDiagnostics(
				pullErr.Result.PullDiagnostics,
				pullsupport.PullDiagnosticsFromError(pullErr.Cause),
			).WithDefaults(
				pullErr.Result.ModelName,
				pullErr.Result.SourceID,
				pullErr.Result.Revision,
				"",
				"pull model",
			)
			return newModelsRootError(
				modelsRootPullFailedCode,
				factoryapi.ErrorFamilyBadRequest,
				pullErr.Error(),
				pullErr,
				pullsupport.NewPullDiagnosticsError(diagnostics, pullErr),
			)
		}
		return err
	}
}

func modelCacheNotFoundMessage(err error) string {
	modelName := modelNameFromError(err, modelinference.ErrModelCacheNotFound.Error())
	if modelName == "" {
		return modelsRootMissingCachePrefix + " <model> first"
	}
	return fmt.Sprintf("%s %s first", modelsRootMissingCachePrefix, modelName)
}

func modelNotFoundMessage(err error) string {
	modelName := modelNameFromError(err, modelinference.ErrNotFound.Error())
	if modelName == "" {
		return modelinference.ErrNotFound.Error()
	}
	return fmt.Sprintf("%s: %s", modelinference.ErrNotFound, modelName)
}

func modelNameFromError(err error, marker string) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	markerIndex := strings.LastIndex(message, marker)
	if markerIndex < 0 {
		return ""
	}
	modelName := strings.TrimSpace(message[markerIndex+len(marker):])
	modelName = strings.TrimPrefix(modelName, ":")
	return strings.TrimSpace(modelName)
}
