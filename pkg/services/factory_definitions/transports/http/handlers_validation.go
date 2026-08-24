package http

import (
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ValidateFactory handles POST /factory-validations by decoding the request and
// invoking the injected Definitions validation operation or root adapter.
func (s *Server) ValidateFactory(w http.ResponseWriter, r *http.Request) {
	decoded, err := decodeJSONWithDiagnostics[factoryapi.Factory](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	req := decoded.Value

	validation, ok := s.requireSubmittedDefinitionValidation(w)
	if !ok {
		return
	}
	if s.guardDefinitionsRequestContext(w, r) {
		return
	}

	result, err := validationentry.ValidateFactoryAPI(r.Context(), req, validation)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		if s.writeDefinitionsRootError(w, err) {
			return
		}
		s.writeError(w, http.StatusBadRequest, invalidRequestPayloadMessage, "BAD_REQUEST")
		return
	}

	s.writeCompatibilityWarning(w, "validate_factory", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, apisurface.FactoryValidationResultToAPI(result))
}
