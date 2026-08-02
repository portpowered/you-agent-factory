package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	"go.uber.org/zap"
)

// ValidateFactory handles POST /factory-validations using validationentry.ValidateFactoryAPI
// with ProfileTopology (structural checks only; no canonical JSON load).
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

	result, err := validationentry.ValidateFactoryAPI(r.Context(), req, s.factoryValidation)
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
	req, err := decodeStrictJSON[factoryapi.FactoryPreviewRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

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
	s.writeJSON(w, http.StatusOK, result)
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.Factory, error) {
	return decodeStrictJSON[factoryapi.Factory](body)
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

	if s.sessionsRoot != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		projection, err := s.sessionsRoot.GetFactorySession(r.Context(), decodeGetFactorySessionRequest(sessionID))
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

func (s *Server) GetFactorySessionResults(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.GetFactorySessionResultsParams) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}

	getter, ok := s.requireDurableSessionResultGetter(w)
	if !ok {
		return
	}
	raw, err := factorysession.ResultRequestFromAPI(params)
	if err == nil {
		raw, err = s.sessionRequests.PrepareResult(raw)
	}
	if err != nil {
		if status, errResp, handled := factorysession.ExecutionErrorResponse(err); handled {
			s.writeJSON(w, status, errResp)
			return
		}
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	response, err := getter.GetDurableFactorySessionResult(r.Context(), string(sessionID), raw)
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
	req, err := decodeOpenFactorySessionRequest(r.Body)
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
			apisurface.FactoryValidationTargetToAPI(interfaces.FactorySessionFieldValidationTarget("required", "folderPath", "folderPath is required")),
		})
		return
	}

	if s.sessionsRoot != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		result, err := s.sessionsRoot.OpenFactorySession(r.Context(), factorysession.OpenRequestFromAPI(req))
		if err != nil {
			s.writeOpenFactorySessionRejected(w, err)
			return
		}
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
	if s.sessionsRoot != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		if err := s.sessionsRoot.CloseFactorySession(r.Context(), sessionID); err != nil {
			if s.writeSessionsRootError(w, sessionID, err) {
				return
			}
			s.logger.Error("close factory session failed", zap.Error(err), zap.String("session_id", sessionID))
			s.writeSessionsRootErrorOrInternal(w, sessionID, err, "failed to close factory session")
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
		s.logger.Error("close factory session failed", zap.Error(err), zap.String("session_id", sessionID))
		s.writeError(w, http.StatusInternalServerError, "failed to close factory session", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeOpenFactorySessionBody(body io.Reader) (factoryapi.OpenFactorySessionJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.OpenFactorySessionJSONRequestBody](body)
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
	if s.durableProjection == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session dispatch read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableProjection, true
}

func (s *Server) requireDurableSessionArtifactReader(w http.ResponseWriter) (durableSessionArtifactReader, bool) {
	if s.durableProjection == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session artifact read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableProjection, true
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
	raw, err := decodeStartFactorySessionRequest(r.Body, s.sessionRequests)
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
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	raw, err := decodeStartFactorySessionRequest(r.Body, s.sessionRequests)
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
	s.writeJSON(w, http.StatusOK, response)
}

type durableSessionGetter interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
}

type durableSessionResultGetter interface {
	GetDurableFactorySessionResult(
		ctx context.Context,
		sessionID string,
		request factorysessionexecution.ResultRequest,
	) (factoryapi.FactorySessionResult, error)
}

type durableSessionEventsReader interface {
	ReadDurableFactorySessionEvents(
		ctx context.Context,
		sessionID string,
		request factorysessionexecution.EventReconnectRequest,
	) (*interfaces.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(
		ctx context.Context,
		sessionID string,
		request factorysessionexecution.EventReconnectRequest,
	) error
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

func (s *Server) requireDurableSessionResultGetter(w http.ResponseWriter) (durableSessionResultGetter, bool) {
	if s.durableProjection == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session result read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableProjection, true
}

func (s *Server) requireDurableSessionEventsReader(w http.ResponseWriter) (durableSessionEventsReader, bool) {
	if s.durableProjection == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session event replay is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableProjection, true
}

func (s *Server) requireDurableSessionResponseEventsReader(w http.ResponseWriter) (durableSessionResponseEventsReader, bool) {
	if s.durableProjection == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session response-event replay is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableProjection, true
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
		live = ReadProjectionSessionListReader{Reader: s.sessionsRoot}
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
