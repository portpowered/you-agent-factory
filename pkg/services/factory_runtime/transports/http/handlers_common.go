package http

import (
	"encoding/json"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func (a *Adapter) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *Adapter) writeError(w http.ResponseWriter, status int, message, code string) {
	a.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
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
