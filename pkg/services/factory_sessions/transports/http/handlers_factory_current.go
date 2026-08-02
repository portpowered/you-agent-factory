package http

import (
	"errors"
	"io"
	"net/http"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func (s *Server) GetCurrentFactory(w http.ResponseWriter, r *http.Request) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, namedFactory)
}

func (s *Server) GetCurrentFactoryBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	definitions, ok := s.requireFactoryDefinitionAPI(w)
	if !ok {
		return
	}
	namedFactory, err := definitions.GetCurrentFactoryForSession(r.Context(), string(sessionID))
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrFactorySessionNotFound):
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current factory not found.", "NOT_FOUND")
			return
		default:
			s.logger.Error("get current factory failed", zap.Error(err), zap.String("session_id", string(sessionID)))
			s.writeError(w, http.StatusInternalServerError, "failed to load current factory", "INTERNAL_ERROR")
			return
		}
	}
	s.writeJSON(w, http.StatusOK, namedFactory)
}

func (s *Server) GetCurrentFactoryWorkstationPromptTemplateContractBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, workstationName string) {
	namedFactory, ok := s.loadCurrentFactoryBySession(w, r, sessionID)
	if !ok {
		return
	}
	workstation, ok := currentFactoryWorkstation(namedFactory, workstationName)
	if !ok {
		s.writeError(w, http.StatusNotFound, "Current factory workstation not found.", "NOT_FOUND")
		return
	}

	if s.workerPrompts == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker prompt service is unavailable", "INTERNAL_ERROR")
		return
	}
	contract := s.workerPrompts.BuildPromptTemplateContract(
		len(workstation.Inputs),
		currentFactoryBundledDocTargetPaths(namedFactory),
	)
	s.writeJSON(w, http.StatusOK, promptTemplateContractResponse(contract))
}

func (s *Server) ValidateCurrentFactoryWorkstationPromptTemplateBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, workstationName string) {
	namedFactory, ok := s.loadCurrentFactoryBySession(w, r, sessionID)
	if !ok {
		return
	}
	workstation, ok := currentFactoryWorkstation(namedFactory, workstationName)
	if !ok {
		s.writeError(w, http.StatusNotFound, "Current factory workstation not found.", "NOT_FOUND")
		return
	}
	req, err := decodePromptTemplateValidationRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	docPaths := currentFactoryBundledDocTargetPaths(namedFactory)
	if s.workerPrompts == nil {
		s.writeError(w, http.StatusInternalServerError, "Worker prompt service is unavailable", "INTERNAL_ERROR")
		return
	}
	result := s.workerPrompts.ValidatePromptTemplate(
		req.Prompt,
		len(workstation.Inputs),
		docPaths,
	)
	s.writeJSON(w, http.StatusOK, promptTemplateValidationResultResponse(result))
}

func (s *Server) loadCurrentFactory(w http.ResponseWriter, r *http.Request) (factoryapi.Factory, bool) {
	namedFactory, err := s.runtime.GetCurrentFactory(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current factory not found.", "NOT_FOUND")
			return factoryapi.Factory{}, false
		default:
			s.logger.Error("get current factory failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load current factory", "INTERNAL_ERROR")
			return factoryapi.Factory{}, false
		}
	}
	return namedFactory, true
}

func (s *Server) loadCurrentFactoryBySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) (factoryapi.Factory, bool) {
	definitions, ok := s.requireFactoryDefinitionAPI(w)
	if !ok {
		return factoryapi.Factory{}, false
	}
	namedFactory, err := definitions.GetCurrentFactoryForSession(r.Context(), string(sessionID))
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrFactorySessionNotFound):
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return factoryapi.Factory{}, false
		case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current factory not found.", "NOT_FOUND")
			return factoryapi.Factory{}, false
		default:
			s.logger.Error("get current factory failed", zap.Error(err), zap.String("session_id", string(sessionID)))
			s.writeError(w, http.StatusInternalServerError, "failed to load current factory", "INTERNAL_ERROR")
			return factoryapi.Factory{}, false
		}
	}
	return namedFactory, true
}

func (s *Server) SaveCurrentFactoryBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	definitions, ok := s.requireFactoryDefinitionAPI(w)
	if !ok {
		return
	}
	req, err := decodeSaveCurrentFactoryBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, message, "BAD_REQUEST", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget())})
			return
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget())})
		return
	}

	mode := factoryapi.FactorySaveModeReplaceCurrent
	if req.Mode != nil {
		mode = *req.Mode
	}

	saved, err := definitions.SaveFactoryForSession(r.Context(), string(sessionID), mode, req.Factory)
	if err != nil {
		s.writeCurrentFactoryError(w, err, "save", zap.String("session_id", string(sessionID)))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}

func (s *Server) writeCurrentFactoryError(
	w http.ResponseWriter,
	err error,
	action string,
	fields ...zap.Field,
) {
	var topologyErr *apisurface.TopologyValidationError
	var domainTopologyErr *interfaces.ValidationTopologyError
	switch {
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
		s.writeError(w, http.StatusNotFound, "Current factory not found.", "NOT_FOUND")
		return
	case errors.Is(err, apisurface.ErrInvalidNamedFactoryName):
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.InvalidFactoryNameValidationTarget())})
		return
	case errors.Is(err, apisurface.ErrFactoryVersionStale):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory definition is stale. Refresh the graph before saving.", "STALE_FACTORY_VERSION", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.StaleFactoryVersionValidationTarget())})
		return
	case errors.As(err, &topologyErr):
		targets := topologyErr.Targets
		if len(targets) == 0 {
			targets = []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget())}
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", targets)
		return
	case errors.As(err, &domainTopologyErr):
		targets := apisurface.FactoryValidationTargetsToAPI(domainTopologyErr.Targets)
		if len(targets) == 0 {
			targets = []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget())}
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", targets)
		return
	case errors.Is(err, apisurface.ErrInvalidNamedFactory):
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FormFactoryPayloadValidationTarget())})
		return
	case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory runtime must be idle before activation.", "FACTORY_NOT_IDLE", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(interfaces.FactoryRuntimeNotIdleValidationTarget())})
		return
	case errors.Is(err, interfaces.ErrNamedFactoryAlreadyExists):
		s.writeError(w, http.StatusConflict, "Named factory already exists.", "FACTORY_ALREADY_EXISTS")
		return
	default:
		logFields := append([]zap.Field{zap.String("action", action)}, fields...)
		s.logger.Error("current factory request failed", append(logFields, zap.Error(err))...)
		if action == "get" {
			s.writeError(w, http.StatusInternalServerError, "failed to load current factory", "INTERNAL_ERROR")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "failed to save current factory", "INTERNAL_ERROR")
		return
	}
}

func decodeSaveCurrentFactoryBody(body io.Reader) (factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody](body)
}

func decodePromptTemplateValidationRequestBody(body io.Reader) (factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateBySessionIdJSONRequestBody](body)
}

func currentFactoryBundledDocTargetPaths(factory factoryapi.Factory) []string {
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		return nil
	}

	paths := make([]string, 0, len(*factory.SupportingFiles.BundledFiles))
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.Type != factoryapi.BundledFileTypeDOC {
			continue
		}
		if bundledFile.TargetPath == "" {
			continue
		}
		paths = append(paths, bundledFile.TargetPath)
	}

	return paths
}

func currentFactoryWorkstation(factory factoryapi.Factory, workstationName string) (factoryapi.Workstation, bool) {
	if factory.Workstations == nil {
		return factoryapi.Workstation{}, false
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name == workstationName || stringValue(workstation.Id) == workstationName {
			return workstation, true
		}
	}
	return factoryapi.Workstation{}, false
}

func promptTemplateContractResponse(contract workers.PromptTemplateContract) factoryapi.PromptTemplateContract {
	availableVariables := make([]factoryapi.PromptTemplateVariableReference, 0, len(contract.AvailableVariables))
	for _, reference := range contract.AvailableVariables {
		availableVariables = append(availableVariables, factoryapi.PromptTemplateVariableReference{
			Category:    factoryapi.PromptTemplateVariableReferenceCategory(reference.Category),
			Description: reference.Description,
			Example:     reference.Example,
			Path:        reference.Path,
		})
	}
	unavailablePatterns := make([]factoryapi.PromptTemplateUnavailableAccessPattern, 0, len(contract.UnavailableAccessPatterns))
	for _, pattern := range contract.UnavailableAccessPatterns {
		unavailablePatterns = append(unavailablePatterns, factoryapi.PromptTemplateUnavailableAccessPattern{
			Example: pattern.Example,
			Path:    pattern.Path,
			Reason:  pattern.Reason,
		})
	}
	return factoryapi.PromptTemplateContract{
		AvailableVariables:        availableVariables,
		InputCount:                contract.InputCount,
		UnavailableAccessPatterns: unavailablePatterns,
	}
}

func promptTemplateValidationResultResponse(result workers.PromptTemplateValidationResult) factoryapi.PromptTemplateValidationResult {
	diagnostics := make([]factoryapi.PromptTemplateDiagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		diagnostics = append(diagnostics, factoryapi.PromptTemplateDiagnostic{
			EndOffset:   diagnostic.EndOffset,
			Kind:        factoryapi.PromptTemplateDiagnosticKind(diagnostic.Kind),
			Message:     diagnostic.Message,
			Path:        diagnostic.Path,
			SourceText:  diagnostic.SourceText,
			StartOffset: diagnostic.StartOffset,
		})
	}
	return factoryapi.PromptTemplateValidationResult{
		Diagnostics: diagnostics,
		Valid:       result.Valid,
	}
}
