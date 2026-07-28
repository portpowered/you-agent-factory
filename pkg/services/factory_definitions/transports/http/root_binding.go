package http

import (
	"context"
	"encoding/json"
	"net/http"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// DefinitionsRoot is the accepted Factory Definitions root contract used by the
// HTTP adapter. Adapter-owned operations invoke this surface rather than
// Definitions internal packages.
type DefinitionsRoot = factorydefinitions.Service

// RootBinding binds the HTTP adapter to one injected Definitions root.
type RootBinding struct {
	Definitions DefinitionsRoot
	Validation  factorydefinitions.SubmittedDefinitionValidationOperation
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Definitions root. Tests inject a focused fake implementing DefinitionsRoot
// without constructing real catalog, authoring, compile, validate, snapshot,
// distribute, or service-local Wire graphs.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return NewHandler(Dependencies{
		DefinitionsRoot: binding.Definitions,
		Validation:      binding.Validation,
	}, logger)
}

func (s *Server) requireDefinitionsRoot(w http.ResponseWriter) (DefinitionsRoot, bool) {
	if s.definitionsRoot == nil {
		s.writeError(w, http.StatusInternalServerError, "factory definitions API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.definitionsRoot, true
}

func (s *Server) requireSubmittedDefinitionValidation(
	w http.ResponseWriter,
) (factorydefinitions.SubmittedDefinitionValidationOperation, bool) {
	if s.validation != nil {
		return s.validation, true
	}
	if s.definitionsRoot == nil {
		s.writeError(w, http.StatusInternalServerError, "factory definition validation is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return submittedDefinitionValidationFromRoot{s.definitionsRoot}, true
}

type submittedDefinitionValidationFromRoot struct {
	root DefinitionsRoot
}

func (adapter submittedDefinitionValidationFromRoot) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if request.Config == nil {
		return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	payload, err := json.Marshal(request.Config)
	if err != nil {
		return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	result, err := adapter.root.ValidateStructuralFactoryDefinition(
		ctx,
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
