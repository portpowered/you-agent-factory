package http

import (
	"encoding/json"
	"io"
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
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

	if s.guardDefinitionsRequestContext(w, r) {
		return
	}

	result, err := s.validateFactoryRequest(r, req)
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

// validateFactoryRequest sends the decoded representation through the single
// Definitions root operation. The legacy validation callback is retained only
// for focused adapter tests and older embedding callers; production root
// composition never constructs that parallel policy path.
func (s *Server) validateFactoryRequest(
	r *http.Request,
	factory factoryapi.Factory,
) (factorydefinitions.ValidationResult, error) {
	if s != nil && s.definitionsRoot != nil {
		payload, err := json.Marshal(factory)
		if err != nil {
			return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
		}
		result, err := s.definitionsRoot.ValidateStructuralFactoryDefinition(
			r.Context(),
			factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
				Canonical: payload,
				Profile:   factorydefinitions.ValidationProfileTopology,
			},
		)
		if err != nil {
			return factorydefinitions.ValidationResult{}, err
		}
		return result.Validation, nil
	}
	if s == nil || s.validation == nil {
		return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	validation := s.validation
	config, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	return validation.ValidateSubmittedDefinition(
		r.Context(),
		factorydefinitions.SubmittedDefinitionValidationRequest{Config: &config},
	)
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.Factory, error) {
	return decodeStrictJSON[factoryapi.Factory](body)
}
