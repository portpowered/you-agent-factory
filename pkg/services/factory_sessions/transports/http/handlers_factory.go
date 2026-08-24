package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinitionentry"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// ValidateFactory handles POST /factory-validations using factorydefinitionentry.ValidateFactoryAPI
// with ProfileTopology (structural checks only; no canonical JSON load).
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

	result, err := factorydefinitionentry.ValidateFactoryAPI(r.Context(), req, s.factoryValidation)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	s.writeCompatibilityWarning(w, "validate_factory", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, apisurface.FactoryValidationResultToAPI(result))
}

// PreviewFactory handles POST /factories/preview using canonical Factory preview semantics.
func (s *Server) PreviewFactory(w http.ResponseWriter, r *http.Request) {
	decoded, err := decodeJSONWithDiagnostics[factoryapi.FactoryPreviewRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	req := decoded.Value

	previewInput, err := apisurface.FactoryPreviewInputFromAPI(req)
	if err != nil {
		var validationErr *apisurface.RequestValidationError
		if errors.As(err, &validationErr) {
			s.writeError(w, http.StatusBadRequest, validationErr.Error(), "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.workflowPreview == nil {
		s.writeError(w, http.StatusInternalServerError, "workflow preview is unavailable", "INTERNAL_ERROR")
		return
	}
	preview, err := s.workflowPreview.PreviewWorkflow(r.Context(), previewInput)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	result := apisurface.FactoryPreviewResultFromPreview(preview)
	s.writeCompatibilityWarning(w, "preview_factory", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.LiveSessionAPI, bool) {
	if s.sessions == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessions, true
}

func (s *Server) requireFactoryDefinitionAPI(w http.ResponseWriter) (apisurface.FactorySaveAPI, bool) {
	if s.factoryDefinitions == nil {
		s.writeError(w, http.StatusInternalServerError, "factory definition API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.factoryDefinitions, true
}

func (s *Server) ListFactorySessions(w http.ResponseWriter, r *http.Request, params factoryapi.ListFactorySessionsParams) {
	raw, err := decodeListFactorySessionsRequest(params, s.sessionRequests)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	if s.sessionsRoot != nil && s.guardSessionsRequestContext(w, r) {
		return
	}

	response, err := s.mergeScopedFactorySessionList(r.Context(), raw)
	if err != nil {
		if errors.Is(err, ErrDurableReaderRequired) {
			s.writeError(w, http.StatusNotImplemented, "durable factory session listing is not implemented", "INTERNAL_ERROR")
			return
		}
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

	if s.liveControl != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		projection, err := s.liveControl.GetFactorySession(r.Context(), decodeGetFactorySessionRequest(sessionID))
		if err != nil {
			if s.writeSessionsRootError(w, string(sessionID), err) {
				return
			}
			s.logger.Error("get factory session failed", zap.Error(err))
			s.writeSessionsRootErrorOrInternal(w, string(sessionID), err, "failed to get factory session")
			return
		}
		s.writeJSON(w, http.StatusOK, factorysession.SessionResponseToAPI(projection))
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
		req factorysessionexecution.InterruptDispatchRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.InterruptDurableFactorySessionDispatch(r.Context(), string(sessionID), req)
	})
}

func (s *Server) OpenFactorySession(w http.ResponseWriter, r *http.Request) {
	if !requestAcceptsJSONContentType(r.Header.Get("Content-Type")) {
		s.writeUnsupportedMediaTypeError(w)
		return
	}
	decoded, err := decodeJSONWithDiagnostics[factoryapi.OpenFactorySessionJSONRequestBody](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	req := decoded.Value
	if strings.TrimSpace(req.FolderPath) == "" {
		s.writeErrorWithTargets(w, http.StatusBadRequest, "folderPath is required", "BAD_REQUEST", []factoryapi.FactoryValidationTarget{
			apisurface.FactoryValidationTargetToAPI(interfaces.FactorySessionFieldValidationTarget("required", "folderPath", "folderPath is required")),
		})
		return
	}

	if s.liveControl != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		result, err := s.liveControl.OpenFactorySession(r.Context(), factorysession.OpenRequestFromAPI(req))
		if err != nil {
			s.writeOpenFactorySessionRejected(w, err)
			return
		}
		s.writeCompatibilityWarning(w, "open_factory_session", decoded.Diagnostics.Paths())
		s.writeJSON(w, http.StatusOK, factorysession.OpenResultToAPI(result))
		return
	}

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.OpenFactorySession(r.Context(), req)
	if err != nil {
		s.writeOpenFactorySessionRejected(w, err)
		return
	}
	s.writeCompatibilityWarning(w, "open_factory_session", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) writeOpenFactorySessionRejected(w http.ResponseWriter, err error) {
	if s.writeSessionsRequestContextOutcome(w, err) {
		return
	}
	s.logger.Debug("open factory session rejected", zap.Error(err))
	var domainTargetedErr interface {
		error
		ErrorTargets() []interfaces.ValidationTarget
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
}

func (s *Server) CloseFactorySession(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.liveControl != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		deletion := s.sessionDeletion
		if deletion == nil {
			deletion, _ = s.liveControl.(factorysessionexecution.LiveDeletionService)
		}
		if deletion == nil {
			s.writeError(w, http.StatusInternalServerError, "factory session deletion is unavailable", "INTERNAL_ERROR")
			return
		}
		if err := deletion.DeleteFactorySession(r.Context(), sessionID); err != nil {
			if s.writeSessionsRootError(w, sessionID, err) {
				return
			}
			s.logger.Error("delete factory session failed", zap.Error(err), zap.String("session_id", sessionID))
			s.writeSessionsRootErrorOrInternal(w, sessionID, err, "failed to delete factory session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	if err := sessionRuntime.CloseFactorySession(r.Context(), sessionID); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("delete factory session failed", zap.Error(err), zap.String("session_id", sessionID))
		s.writeError(w, http.StatusInternalServerError, "failed to delete factory session", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireDurableExecutionAPI(w http.ResponseWriter) (apisurface.DurableSessionExecutionAPI, bool) {
	if s.durableExecution == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableExecution, true
}

func (s *Server) writeDurableExecutionError(w http.ResponseWriter, err error) bool {
	if status, response, ok := factorysession.ExecutionErrorResponse(err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
	raw, diagnostics, err := decodeStartFactorySessionRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		result, err := s.sessionsRoot.StartAsync(r.Context(), raw)
		if err != nil {
			if s.writeSessionsRootError(w, "", err) {
				return
			}
			s.logger.Error("durable factory session async start failed", zap.Error(err))
			s.writeSessionsRootErrorOrInternal(w, "", err, "durable factory session execution failed")
			return
		}
		s.writeCompatibilityWarning(w, "start_durable_factory_session_async", diagnostics.Paths())
		s.writeJSON(w, http.StatusOK, factorysession.AsyncStartResponseToAPI(result))
		return
	}

	execution, ok := s.requireDurableExecutionAPI(w)
	if !ok {
		return
	}
	response, err := execution.StartDurableFactorySessionAsync(r.Context(), raw)
	if err != nil {
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeCompatibilityWarning(w, "start_durable_factory_session_async", diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	raw, diagnostics, err := decodeStartFactorySessionRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		result, err := s.sessionsRoot.StartSync(r.Context(), raw)
		if err != nil {
			if s.writeSessionsRootError(w, "", err) {
				return
			}
			s.logger.Error("durable factory session sync start failed", zap.Error(err))
			s.writeSessionsRootErrorOrInternal(w, "", err, "durable factory session execution failed")
			return
		}
		s.writeCompatibilityWarning(w, "start_durable_factory_session_sync", diagnostics.Paths())
		s.writeJSON(w, http.StatusOK, factorysession.SyncStartResponseToAPI(result))
		return
	}

	execution, ok := s.requireDurableExecutionAPI(w)
	if !ok {
		return
	}
	response, err := execution.StartDurableFactorySessionSync(r.Context(), raw)
	if err != nil {
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeCompatibilityWarning(w, "start_durable_factory_session_sync", diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, response)
}

type durableSessionGetter interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
}

type durableSessionResponseEventsReader interface {
	SubscribeDurableFactoryResponseEvents(
		ctx context.Context,
		request factorysessionexecution.ResponseEventSubscriptionRequest,
	) (apisurface.FactoryResponseEventSubscription, error)
}

type DurableExecutionSessionLister interface {
	ListSessions(
		context.Context,
		factorysessionexecution.ListSessionsRequest,
	) (factorysessionexecution.ListSessionsResult, error)
}

func (s *Server) requireDurableSessionGetter(w http.ResponseWriter) (durableSessionGetter, bool) {
	if s.durableLifecycle == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableLifecycle, true
}

func (s *Server) requireDurableSessionResponseEventsReader(w http.ResponseWriter) (durableSessionResponseEventsReader, bool) {
	if s.durableResponseEvents == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session response-event replay is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableResponseEvents, true
}

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func (s *Server) mergeScopedFactorySessionList(
	ctx context.Context,
	normalized factorysessionexecution.ListSessionsRequest,
) (factoryapi.ListFactorySessionsResponse, error) {
	var live scopedLiveReader
	var durable scopedDurableReader

	if s.sessionsRoot != nil {
		live = ReadProjectionSessionListReader{Reader: s.liveControl}
		durable = s.sessionsRoot
	} else {
		live = s.liveSessionLister
		durable = s.durableLister
	}

	result, err := mergeScopedSessionList(ctx, normalized, live, durable)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ScopedSessionListResponseToAPI(result), nil
}

func (s *Server) writeDurableSessionReadError(w http.ResponseWriter, err error) bool {
	return s.writeSessionsRootError(w, "", err)
}

func (s *Server) writeDurableSessionListError(w http.ResponseWriter, err error) {
	if s.writeDurableSessionReadError(w, err) {
		return
	}
	s.logger.Error("list durable factory sessions failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
}
