package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	"go.uber.org/zap"
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
	return decodeStrictJSON[factoryapi.Factory](body)
}

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.SessionAPISurface, bool) {
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionRuntime, true
}

func (s *Server) ListFactorySessions(w http.ResponseWriter, r *http.Request) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.ListFactorySessions(r.Context())
	if err != nil {
		s.logger.Error("list factory sessions failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) OpenFactorySession(w http.ResponseWriter, r *http.Request) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	req, err := decodeOpenFactorySessionBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.FolderPath) == "" {
		s.writeErrorWithTargets(w, http.StatusBadRequest, "folderPath is required", "BAD_REQUEST", []factoryapi.FactoryValidationTarget{
			factoryvalidation.FactorySessionFieldTarget("required", "folderPath", "folderPath is required"),
		})
		return
	}
	response, err := sessionRuntime.OpenFactorySession(r.Context(), req)
	if err != nil {
		s.logger.Debug("open factory session rejected", zap.Error(err))
		var targetedErr interface {
			error
			ErrorTargets() []factoryapi.FactoryValidationTarget
		}
		if errors.As(err, &targetedErr) {
			s.writeErrorWithTargets(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST", targetedErr.ErrorTargets())
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) CloseFactorySession(w http.ResponseWriter, r *http.Request, sessionID string) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	if err := sessionRuntime.CloseFactorySession(r.Context(), sessionID); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("close factory session failed", zap.Error(err), zap.String("session_id", sessionID))
		s.writeError(w, http.StatusInternalServerError, "failed to close factory session", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetCurrentFactory(w http.ResponseWriter, r *http.Request) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, namedFactory)
}

func (s *Server) GetCurrentFactoryBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	namedFactory, err := sessionRuntime.GetCurrentFactoryForSession(r.Context(), string(sessionID))
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

	contract := workerprompting.BuildPromptTemplateContract(len(workstation.Inputs))
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

	result := workerprompting.ValidatePromptTemplate(req.Prompt, len(workstation.Inputs))
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
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return factoryapi.Factory{}, false
	}
	namedFactory, err := sessionRuntime.GetCurrentFactoryForSession(r.Context(), string(sessionID))
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
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	req, err := decodeSaveCurrentFactoryBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, message, "BAD_REQUEST", []factoryapi.FactoryValidationTarget{factoryvalidation.FormFactoryPayloadTarget()})
			return
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST", []factoryapi.FactoryValidationTarget{factoryvalidation.FormFactoryPayloadTarget()})
		return
	}

	mode := factoryapi.FactorySaveModeReplaceCurrent
	if req.Mode != nil {
		mode = *req.Mode
	}

	saved, err := sessionRuntime.SaveFactoryForSession(r.Context(), string(sessionID), mode, req.Factory)
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
	switch {
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
		s.writeError(w, http.StatusNotFound, "Current factory not found.", "NOT_FOUND")
		return
	case errors.Is(err, apisurface.ErrInvalidNamedFactoryName):
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME", []factoryapi.FactoryValidationTarget{factoryvalidation.InvalidFactoryNameTarget()})
		return
	case errors.Is(err, apisurface.ErrFactoryVersionStale):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory definition is stale. Refresh the graph before saving.", "STALE_FACTORY_VERSION", []factoryapi.FactoryValidationTarget{factoryvalidation.StaleFactoryVersionTarget()})
		return
	case errors.As(err, &topologyErr):
		targets := topologyErr.Targets
		if len(targets) == 0 {
			targets = []factoryapi.FactoryValidationTarget{factoryvalidation.FormFactoryPayloadTarget()}
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", targets)
		return
	case errors.Is(err, apisurface.ErrInvalidNamedFactory):
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", []factoryapi.FactoryValidationTarget{factoryvalidation.FormFactoryPayloadTarget()})
		return
	case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory runtime must be idle before activation.", "FACTORY_NOT_IDLE", []factoryapi.FactoryValidationTarget{factoryvalidation.FactoryRuntimeNotIdleTarget()})
		return
	case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
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

func decodeOpenFactorySessionBody(body io.Reader) (factoryapi.OpenFactorySessionJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.OpenFactorySessionJSONRequestBody](body)
}

func decodeSaveCurrentFactoryBody(body io.Reader) (factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody](body)
}

func decodePromptTemplateValidationRequestBody(body io.Reader) (factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateBySessionIdJSONRequestBody](body)
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

func promptTemplateContractResponse(contract workerprompting.PromptTemplateContract) factoryapi.PromptTemplateContract {
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

func promptTemplateValidationResultResponse(result workerprompting.PromptTemplateValidationResult) factoryapi.PromptTemplateValidationResult {
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
