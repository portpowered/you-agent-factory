package http

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	"go.uber.org/zap"
)

// ValidateFactory handles POST /factory-validations using validationentry.ValidateFactoryAPI
// with ProfileTopology (structural checks only; no canonical JSON load).
func (s *Server) ValidateFactory(w http.ResponseWriter, r *http.Request) {
	req, err := decodeNamedFactoryBody(r.Body)
	if err != nil {
		if target, ok := layoutRequestValidationTarget(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST", []factoryapi.FactoryValidationTarget{target})
			return
		}
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	result, err := validationentry.ValidateFactoryAPI(r.Context(), req, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	s.writeJSON(w, http.StatusOK, apisurface.FactoryValidationResultToAPI(result))
}

// PreviewFactory handles POST /factories/preview using canonical Factory preview semantics.
func (s *Server) PreviewFactory(w http.ResponseWriter, r *http.Request) {
	s.writeFactoryPreview(w, r, false)
}

// PreviewWorkflow handles POST /workflow-previews as an obsolete compatibility alias of Factory preview.
func (s *Server) PreviewWorkflow(w http.ResponseWriter, r *http.Request) {
	s.writeFactoryPreview(w, r, true)
}

func (s *Server) writeFactoryPreview(w http.ResponseWriter, r *http.Request, compatibility bool) {
	if compatibility {
		w.Header().Set("Deprecation", "true")
		w.Header().Set("Link", `</factories/preview>; rel="successor-version"`)
	}

	req, err := decodeStrictJSON[factoryapi.FactoryPreviewRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	previewInput, err := apisurface.FactoryPreviewRequestFromAPI(req)
	if err != nil {
		var validationErr *apisurface.RequestValidationError
		if errors.As(err, &validationErr) {
			s.writeError(w, http.StatusBadRequest, validationErr.Error(), "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	result := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(previewInput))
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.SessionAPISurface, bool) {
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionRuntime, true
}

func (s *Server) ListFactorySessions(w http.ResponseWriter, r *http.Request, params factoryapi.ListFactorySessionsParams) {
	normalized, err := factorysession.ListSessionsRequestFromAPI(params)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	if normalized.Scope == factorysessionexecution.SessionListScopePersisted {
		if _, ok := s.runtime.(durableExecutionSessionLister); !ok {
			s.writeError(w, http.StatusNotImplemented, "durable factory session listing is not implemented", "INTERNAL_ERROR")
			return
		}
	}

	needsWorkspaceLive := normalized.Scope == factorysessionexecution.SessionListScopeLive ||
		normalized.Scope == factorysessionexecution.SessionListScopeAll
	if needsWorkspaceLive && s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return
	}

	response, err := s.mergeScopedFactorySessionList(r.Context(), normalized)
	if err != nil {
		s.writeDurableSessionListError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		getter, ok := s.requireDurableSessionGetter(w)
		if !ok {
			return
		}
		response, err := getter.GetDurableFactorySession(r.Context(), string(sessionID))
		if err != nil {
			if s.writeDurableSessionReadError(w, err) {
				return
			}
			s.logger.Error("get durable factory session failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to get factory session", "INTERNAL_ERROR")
			return
		}
		s.writeJSON(w, http.StatusOK, response)
		return
	}

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.GetFactorySession(r.Context(), string(sessionID))
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get factory session failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySessionResult(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.GetFactorySessionResult(r.Context(), string(sessionID))
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) || errors.Is(err, apisurface.ErrFactorySessionResultUnavailable) {
			s.writeError(w, http.StatusNotFound, "factory session result not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get factory session result failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session result", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySessionResults(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.GetFactorySessionResultsParams) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	getter, ok := s.requireDurableSessionResultGetter(w)
	if !ok {
		return
	}
	response, err := getter.GetDurableFactorySessionResult(r.Context(), string(sessionID), params)
	if err != nil {
		if status, errResp, handled := factorysession.ExecutionErrorResponse(err); handled {
			s.writeJSON(w, status, errResp)
			return
		}
		if s.writeDurableSessionReadError(w, err) {
			return
		}
		s.logger.Error("get durable factory session result failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session result", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) ListFactorySessionDispatches(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListFactorySessionDispatchesParams) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	reader, ok := s.requireDurableSessionDispatchReader(w)
	if !ok {
		return
	}
	response, err := reader.ListDurableFactorySessionDispatches(r.Context(), string(sessionID), params)
	if err != nil {
		if status, errResp, handled := factorysession.ExecutionErrorResponse(err); handled {
			s.writeJSON(w, status, errResp)
			return
		}
		if s.writeDurableSessionReadError(w, err) {
			return
		}
		s.logger.Error("list durable factory session dispatches failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list factory session dispatches", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, dispatchID factoryapi.DispatchID) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	reader, ok := s.requireDurableSessionDispatchReader(w)
	if !ok {
		return
	}
	response, err := reader.GetDurableFactorySessionDispatch(r.Context(), string(sessionID), string(dispatchID))
	if err != nil {
		if s.writeDurableSessionReadError(w, err) {
			return
		}
		s.logger.Error("get durable factory session dispatch failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session dispatch", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) ListFactorySessionArtifacts(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	reader, ok := s.requireDurableSessionArtifactReader(w)
	if !ok {
		return
	}
	response, err := reader.ListDurableFactorySessionArtifacts(r.Context(), string(sessionID))
	if err != nil {
		if s.writeDurableSessionReadError(w, err) {
			return
		}
		s.logger.Error("list durable factory session artifacts failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list factory session artifacts", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySessionArtifact(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, artifactID factoryapi.ArtifactID) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	reader, ok := s.requireDurableSessionArtifactReader(w)
	if !ok {
		return
	}
	response, err := reader.GetDurableFactorySessionArtifact(r.Context(), string(sessionID), string(artifactID))
	if err != nil {
		if s.writeDurableSessionReadError(w, err) {
			return
		}
		s.logger.Error("get durable factory session artifact failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session artifact", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetFactorySessionPartialResult(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.GetFactorySessionPartialResult(r.Context(), string(sessionID))
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) || errors.Is(err, apisurface.ErrFactorySessionResultUnavailable) {
			s.writeError(w, http.StatusNotFound, "factory session partial result not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get factory session partial result failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session partial result", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) InterruptFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableInterruptDispatchControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionInterruptDispatchRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.InterruptDurableFactorySessionDispatch(r.Context(), string(sessionID), req)
	})
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
			apisurface.FactoryValidationTargetToAPI(factoryvalidation.FactorySessionFieldTarget("required", "folderPath", "folderPath is required")),
		})
		return
	}
	response, err := sessionRuntime.OpenFactorySession(r.Context(), req)
	if err != nil {
		s.logger.Debug("open factory session rejected", zap.Error(err))
		var domainTargetedErr interface {
			error
			ErrorTargets() []factoryvalidation.Target
		}
		var targetedErr interface {
			error
			ErrorTargets() []factoryapi.FactoryValidationTarget
		}
		code := "BAD_REQUEST"
		var codedErr interface {
			ErrorCode() string
		}
		if errors.As(err, &codedErr) {
			code = codedErr.ErrorCode()
		}
		if errors.As(err, &domainTargetedErr) {
			s.writeErrorWithTargets(
				w,
				http.StatusBadRequest,
				err.Error(),
				code,
				apisurface.FactoryValidationTargetsToAPI(domainTargetedErr.ErrorTargets()),
			)
			return
		}
		if errors.As(err, &targetedErr) {
			s.writeErrorWithTargets(w, http.StatusBadRequest, err.Error(), code, targetedErr.ErrorTargets())
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error(), code)
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

	contract := workerprompting.BuildPromptTemplateContract(
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
	result := workerprompting.ValidatePromptTemplate(
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
		if target, ok := layoutRequestValidationTarget(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", []factoryapi.FactoryValidationTarget{target})
			return
		}
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, message, "BAD_REQUEST", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.FormFactoryPayloadTarget())})
			return
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.FormFactoryPayloadTarget())})
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
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.InvalidFactoryNameTarget())})
		return
	case errors.Is(err, apisurface.ErrFactoryVersionStale):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory definition is stale. Refresh the graph before saving.", "STALE_FACTORY_VERSION", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.StaleFactoryVersionTarget())})
		return
	case errors.As(err, &topologyErr):
		targets := topologyErr.Targets
		if len(targets) == 0 {
			targets = []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.FormFactoryPayloadTarget())}
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", targets)
		return
	case errors.Is(err, apisurface.ErrInvalidNamedFactory):
		s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.FormFactoryPayloadTarget())})
		return
	case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
		s.writeErrorWithTargets(w, http.StatusConflict, "Current factory runtime must be idle before activation.", "FACTORY_NOT_IDLE", []factoryapi.FactoryValidationTarget{apisurface.FactoryValidationTargetToAPI(factoryvalidation.FactoryRuntimeNotIdleTarget())})
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

type durableSessionDispatchReader interface {
	ListDurableFactorySessionDispatches(
		ctx context.Context,
		sessionID string,
		params factoryapi.ListFactorySessionDispatchesParams,
	) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(
		ctx context.Context,
		sessionID, dispatchID string,
	) (factoryapi.FactoryDispatch, error)
}

type durableSessionArtifactReader interface {
	ListDurableFactorySessionArtifacts(
		ctx context.Context,
		sessionID string,
	) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(
		ctx context.Context,
		sessionID, artifactID string,
	) (factoryapi.FactorySessionArtifactDetail, error)
}

func (s *Server) requireDurableSessionDispatchReader(w http.ResponseWriter) (durableSessionDispatchReader, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session dispatch read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	reader, ok := s.runtime.(durableSessionDispatchReader)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session dispatch read is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return reader, true
}

func (s *Server) requireDurableSessionArtifactReader(w http.ResponseWriter) (durableSessionArtifactReader, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session artifact read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	reader, ok := s.runtime.(durableSessionArtifactReader)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session artifact read is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return reader, true
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

func (s *Server) requireDurableExecutionAPI(w http.ResponseWriter) (apisurface.DurableSessionExecutionAPI, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	execution, ok := s.runtime.(apisurface.DurableSessionExecutionAPI)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session execution is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return execution, true
}

func (s *Server) writeDurableExecutionError(w http.ResponseWriter, err error) bool {
	if status, response, ok := factorysession.ExecutionErrorResponse(err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableExecutionAPI(w)
	if !ok {
		return
	}

	req, err := decodeStrictJSON[factoryapi.FactorySessionExecutionRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	response, err := execution.StartDurableFactorySessionAsync(r.Context(), req)
	if err != nil {
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableExecutionAPI(w)
	if !ok {
		return
	}

	req, err := decodeStrictJSON[factoryapi.FactorySessionExecutionRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	response, err := execution.StartDurableFactorySessionSync(r.Context(), req)
	if err != nil {
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

type durableSessionGetter interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
}

type durableSessionResultGetter interface {
	GetDurableFactorySessionResult(
		ctx context.Context,
		sessionID string,
		params factoryapi.GetFactorySessionResultsParams,
	) (factoryapi.FactorySessionResult, error)
}

type durableSessionEventsReader interface {
	ReadDurableFactorySessionEvents(
		ctx context.Context,
		sessionID string,
		params factoryapi.GetEventsBySessionIdParams,
	) (*interfaces.FactoryEventStream, error)
}

type durableExecutionSessionLister interface {
	ListDurableExecutionSessions(
		context.Context,
		factorysessionexecution.ListSessionsRequest,
	) (factorysessionexecution.ListSessionsResult, error)
}

func (s *Server) requireDurableSessionGetter(w http.ResponseWriter) (durableSessionGetter, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	getter, ok := s.runtime.(durableSessionGetter)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session read is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return getter, true
}

func (s *Server) requireDurableSessionResultGetter(w http.ResponseWriter) (durableSessionResultGetter, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session result read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	getter, ok := s.runtime.(durableSessionResultGetter)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session result retrieval is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return getter, true
}

func (s *Server) requireDurableSessionEventsReader(w http.ResponseWriter) (durableSessionEventsReader, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session event replay is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	reader, ok := s.runtime.(durableSessionEventsReader)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session event replay is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return reader, true
}

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func (s *Server) mergeScopedFactorySessionList(
	ctx context.Context,
	normalized factorysessionexecution.ListSessionsRequest,
) (factoryapi.ListFactorySessionsResponse, error) {
	needsWorkspaceLive := normalized.Scope == factorysessionexecution.SessionListScopeLive ||
		normalized.Scope == factorysessionexecution.SessionListScopeAll
	needsDurable := normalized.Scope == factorysessionexecution.SessionListScopePersisted ||
		normalized.Scope == factorysessionexecution.SessionListScopeAll ||
		normalized.Scope == factorysessionexecution.SessionListScopeLive

	var workspaceSessions []factoryapi.FactorySessionSummary
	if needsWorkspaceLive {
		if s.sessionRuntime == nil {
			return factoryapi.ListFactorySessionsResponse{}, errors.New("session runtime is unavailable")
		}
		liveResponse, err := s.sessionRuntime.ListFactorySessions(ctx)
		if err != nil {
			return factoryapi.ListFactorySessionsResponse{}, err
		}
		workspaceSessions = append([]factoryapi.FactorySessionSummary(nil), liveResponse.Sessions...)
	}

	var durableScoped factorysessionexecution.ListSessionsResult
	if needsDurable {
		if lister, ok := s.runtime.(durableExecutionSessionLister); ok {
			durableResult, err := lister.ListDurableExecutionSessions(ctx, factorysessionexecution.ListSessionsRequest{
				Scope: factorysessionexecution.SessionListScopeAll,
			})
			if err != nil {
				return factoryapi.ListFactorySessionsResponse{}, err
			}
			durableScoped = factorysessionexecution.ApplySessionListScope(factorysessionexecution.ListSessionsResult{
				Scope:           normalized.Scope,
				LiveSessions:    durableResult.LiveSessions,
				DurableSessions: durableResult.DurableSessions,
			}, normalized)
		}
	}

	durableAPI := factorysession.ListSessionsResponseToAPI(durableScoped)
	switch normalized.Scope {
	case factorysessionexecution.SessionListScopePersisted:
		return durableAPI, nil
	default:
		mergedSessions := append([]factoryapi.FactorySessionSummary(nil), workspaceSessions...)
		mergedSessions = append(mergedSessions, durableAPI.Sessions...)
		mergedSessions = sortMergedFactorySessionSummaries(mergedSessions)
		response := durableAPI
		response.Sessions = mergedSessions
		if normalized.Scope == factorysessionexecution.SessionListScopeLive {
			response.DurableSessions = nil
		}
		return response, nil
	}
}

func sortMergedFactorySessionSummaries(sessions []factoryapi.FactorySessionSummary) []factoryapi.FactorySessionSummary {
	if len(sessions) < 2 {
		return sessions
	}
	sorted := append([]factoryapi.FactorySessionSummary(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].Id, sorted[j].Id) < 0
	})
	return sorted
}

func (s *Server) writeDurableSessionReadError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return true
	}
	if errors.Is(err, factorysessionexecution.ErrDispatchNotFound) {
		s.writeError(w, http.StatusNotFound, "factory session dispatch not found", "NOT_FOUND")
		return true
	}
	if errors.Is(err, factorysessionexecution.ErrArtifactNotFound) {
		s.writeError(w, http.StatusNotFound, "factory session artifact not found", "NOT_FOUND")
		return true
	}
	var validationErr *factorysessionexecution.ValidationError
	if errors.As(err, &validationErr) {
		s.writeError(w, http.StatusBadRequest, validationErr.Message, "BAD_REQUEST")
		return true
	}
	return false
}

func (s *Server) writeDurableSessionListError(w http.ResponseWriter, err error) {
	if s.writeDurableSessionReadError(w, err) {
		return
	}
	s.logger.Error("list durable factory sessions failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
}
