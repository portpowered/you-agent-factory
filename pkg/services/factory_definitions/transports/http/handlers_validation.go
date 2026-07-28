package http

import (
	"io"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// ValidateFactory handles POST /factory-validations by decoding the request and
// invoking the injected Definitions validation operation or root adapter.
func (s *Server) ValidateFactory(w http.ResponseWriter, r *http.Request) {
	req, err := decodeNamedFactoryBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	validation, ok := s.requireSubmittedDefinitionValidation(w)
	if !ok {
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

	s.writeJSON(w, http.StatusOK, apisurface.FactoryValidationResultToAPI(result))
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.Factory, error) {
	return decodeStrictJSON[factoryapi.Factory](body)
}
