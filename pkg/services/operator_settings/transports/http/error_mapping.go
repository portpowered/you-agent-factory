package http

import (
	"errors"
	"net/http"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const operatorSettingsInternalFailureMessage = "operator settings request failed"

const (
	operatorSettingsInvalidLoadRequestMessage              = "invalid operator settings load request"
	operatorSettingsInvalidUpdateRequestMessage            = "invalid operator settings update request"
	operatorSettingsDocumentMalformedMessage               = "operator document is malformed"
	operatorSettingsDocumentNotFoundMessage                = "operator document not found"
	operatorSettingsDocumentUnsupportedMessage             = "operator document update is unsupported"
	operatorSettingsDocumentConflictMessage                = "operator document persist conflict"
	operatorSettingsResolutionInvalidInputMessage          = "operator effective resolution input is invalid"
	operatorSettingsResolutionUnsupportedOverrideMessage   = "operator effective resolution override is unsupported"
	operatorSettingsResolutionConflictMessage              = "operator effective resolution conflict"
	operatorSettingsErrorCodeDocumentUnsupported           = "OPERATOR_DOCUMENT_UNSUPPORTED"
	operatorSettingsErrorCodeDocumentConflict              = "OPERATOR_DOCUMENT_CONFLICT"
	operatorSettingsErrorCodeResolutionUnsupportedOverride = "OPERATOR_RESOLUTION_UNSUPPORTED_OVERRIDE"
	operatorSettingsErrorCodeResolutionConflict            = "OPERATOR_RESOLUTION_CONFLICT"
)

// RootErrorResponse maps typed Operator Settings root failures and adapter
// decode validation failures to HTTP status and the public ErrorResponse shape.
// It returns false when err is not a known mapped typed failure.
func RootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	if status, response, ok := settingsRequestContextErrorResponse(err); ok {
		if response == nil {
			return 0, factoryapi.ErrorResponse{}, true
		}
		if errResp, ok := response.(factoryapi.ErrorResponse); ok {
			return status, errResp, true
		}
	}

	if IsLoadDocumentBadRequest(err) {
		return badRequestErrorResponse(operatorSettingsInvalidLoadRequestMessage)
	}
	if IsApplyDocumentUpdateBadRequest(err) {
		return badRequestErrorResponse(operatorSettingsInvalidUpdateRequestMessage)
	}

	switch {
	case errors.Is(err, operatorsettings.ErrDocumentMalformed):
		return badRequestErrorResponse(operatorSettingsDocumentMalformedMessage)
	case errors.Is(err, operatorsettings.ErrDocumentNotFound):
		return notFoundErrorResponse(operatorSettingsDocumentNotFoundMessage)
	case errors.Is(err, operatorsettings.ErrDocumentUnsupported):
		return unsupportedDocumentErrorResponse()
	case errors.Is(err, operatorsettings.ErrDocumentConflict):
		return conflictErrorResponse(operatorSettingsDocumentConflictMessage)
	case errors.Is(err, operatorsettings.ErrResolutionInvalidInput):
		return badRequestErrorResponse(operatorSettingsResolutionInvalidInputMessage)
	case errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride):
		return unsupportedResolutionOverrideErrorResponse()
	case errors.Is(err, operatorsettings.ErrResolutionConflict):
		return resolutionConflictErrorResponse()
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func badRequestErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCodeBADREQUEST,
	}, true
}

func notFoundErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
	}, true
}

func unsupportedDocumentErrorResponse() (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: operatorSettingsDocumentUnsupportedMessage,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCode(operatorSettingsErrorCodeDocumentUnsupported),
	}, true
}

func conflictErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCode(operatorSettingsErrorCodeDocumentConflict),
	}, true
}

func unsupportedResolutionOverrideErrorResponse() (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: operatorSettingsResolutionUnsupportedOverrideMessage,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCode(operatorSettingsErrorCodeResolutionUnsupportedOverride),
	}, true
}

func resolutionConflictErrorResponse() (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: operatorSettingsResolutionConflictMessage,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCode(operatorSettingsErrorCodeResolutionConflict),
	}, true
}

func (a *Adapter) writeRootError(w http.ResponseWriter, err error) bool {
	if a.writeSettingsRequestContextOutcome(w, err) {
		return true
	}
	if status, response, ok := RootErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeRootOrInternalError(w http.ResponseWriter, err error) {
	if a.writeRootError(w, err) {
		return
	}
	a.writeError(
		w,
		http.StatusInternalServerError,
		operatorSettingsInternalFailureMessage,
		string(factoryapi.ErrorResponseCodeINTERNALERROR),
	)
}
