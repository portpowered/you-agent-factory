package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

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

	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	result := factoryvalidation.Validate(&cfg)
	s.writeJSON(w, http.StatusOK, factoryapi.FactoryValidationResult{
		Targets: factoryvalidation.ToValidationTargets(result.Targets),
	})
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.Factory, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.Factory
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return factoryapi.Factory{}, requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return factoryapi.Factory{}, err
	}
	return req, nil
}
