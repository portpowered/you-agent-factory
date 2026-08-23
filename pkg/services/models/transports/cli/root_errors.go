package cli

import (
	"errors"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	modelsRootModelNotFoundCode  = "NOT_FOUND"
	modelsRootCacheNotFoundCode  = "MODEL_CACHE_NOT_FOUND"
	modelsRootCacheInUseCode     = "MODEL_CACHE_IN_USE"
	modelsRootCacheUnsafeCode    = "BAD_REQUEST"
	modelsRootDefaultErrorText   = "models command failed"
	modelsRootMissingCachePrefix = "model cache is not installed; run you models pull"
)

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

func mapModelsRootError(err error) error {
	if err == nil {
		return nil
	}
	switch {
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
		if errors.As(err, &pullErr) {
			return fmt.Errorf(
				"managed runtime pull failed (%s readiness %s)",
				pullErr.Result.ManagedPullOutcome,
				pullErr.Result.ReadinessState,
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
