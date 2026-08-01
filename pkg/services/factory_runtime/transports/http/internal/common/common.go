// Package common contains protocol mechanics shared by the Factory Runtime
// HTTP operation handlers. It deliberately has no dependency on the parent
// adapter so operation packages can remain independently owned.
package common

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ErrRequestBodyRequired is returned when an operation requires a JSON body but
// the request body is empty.
var ErrRequestBodyRequired = errors.New("request body is required")

// ErrRuntimeServiceRequired is returned when an adapter is invoked without its
// required process-scoped Runtime root.
var ErrRuntimeServiceRequired = errors.New("factory runtime service is required")

// RequireRuntimeRoot keeps nil-root handling identical across operation
// packages without creating another dependency-injection path.
func RequireRuntimeRoot(root factoryruntime.Service) (factoryruntime.Service, error) {
	if root == nil {
		return nil, ErrRuntimeServiceRequired
	}
	return root, nil
}

// DecodeRequiredJSON decodes one required JSON request body using the same
// permissive JSON behavior as the original Runtime HTTP adapter.
func DecodeRequiredJSON[T any](body io.Reader) (T, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		var zero T
		return zero, err
	}
	if len(payload) == 0 {
		var zero T
		return zero, ErrRequestBodyRequired
	}
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// DecodeOptionalJSON decodes an optional JSON request body. An empty body
// produces the zero value and is valid.
func DecodeOptionalJSON[T any](body io.Reader) (T, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		var zero T
		return zero, err
	}
	if len(payload) == 0 {
		var zero T
		return zero, nil
	}
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// WriteJSON writes a generated-contract-shaped JSON response.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteError writes a generated ErrorResponse with the family derived from its
// HTTP status.
func WriteError(w http.ResponseWriter, status int, message, code string) {
	WriteJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  ErrorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

// ErrorFamilyForStatus maps the Runtime HTTP status classes to the public
// generated error family.
func ErrorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
