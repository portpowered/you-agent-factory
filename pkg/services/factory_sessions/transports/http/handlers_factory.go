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
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	validationentry "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryvalidation"
	"go.uber.org/zap"
)

// TODO: this should be done under factory validations, why is this here?
// ValidateFactory handles POST /factory-validations using factorydefinitionentry.ValidateFactoryAPI
// with ProfileTopology (structural checks only; no canonical JSON load).
func (s *AuthoringHandler) ValidateFactory(w http.ResponseWriter, r *http.Request) {
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

	result, err := validationentry.ValidateFactoryAPI(r.Context(), req, s.factoryValidation)
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
func (s *AuthoringHandler) PreviewFactory(w http.ResponseWriter, r *http.Request) {
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

func (s *ReadHandler) requireSessionRuntime(w http.ResponseWriter) (apisurface.LiveSessionAPI, bool) {
	if s.sessions == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessions, true
}

func (s *AuthoringHandler) requireFactoryDefinitionAPI(w http.ResponseWriter) (apisurface.FactorySaveAPI, bool) {
	if s.factoryDefinitions == nil {
		s.writeError(w, http.StatusInternalServerError, "factory definition API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.factoryDefinitions, true
}

func (s *ReadHandler) ListFactorySessions(w http.ResponseWriter, r *http.Request, params factoryapi.ListFactorySessionsParams) {
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

func (s *ReadHandler) GetFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		if s.sessionsRoot == nil {
			s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
			return
		}
		result, err := s.sessionsRoot.Get(r.Context(), factorysessionexecution.SessionGetRequest{
			SessionID: string(sessionID), Mode: factorysessionexecution.SessionOperationModeDurable,
		})
		if err != nil {
			if s.writeDurableSessionReadError(w, err) {
				return
			}
			s.logger.Error("get durable factory session failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to get factory session", "INTERNAL_ERROR")
			return
		}
		if result.Durable == nil {
			s.writeError(w, http.StatusInternalServerError, "durable factory session read is unavailable", "INTERNAL_ERROR")
			return
		}
		s.writeJSON(w, http.StatusOK, factorysession.SessionReadResponseToAPI(*result.Durable))
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

func (s *ReadHandler) GetFactorySessionResult(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
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

func (s *ReadHandler) GetFactorySessionPartialResult(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
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

func (s *LifecycleHandler) InterruptFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableInterruptDispatchControl(w, r, sessionID)
}

func (s *LifecycleHandler) OpenFactorySession(w http.ResponseWriter, r *http.Request) {
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

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	start, err := s.sessionsRoot.Start(r.Context(), factorysession.SessionStartRequestFromAPI(req))
	if err != nil {
		s.writeOpenFactorySessionRejected(w, err)
		return
	}
	if start.Live == nil {
		s.logger.Error("canonical factory session start returned no live result")
		s.writeError(w, http.StatusInternalServerError, "failed to open factory session", "INTERNAL_ERROR")
		return
	}
	s.writeCompatibilityWarning(w, "open_factory_session", decoded.Diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, factorysession.SessionOpenResultToAPI(start.Live))
}

func (s *LifecycleHandler) writeOpenFactorySessionRejected(w http.ResponseWriter, err error) {
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

func (s *LifecycleHandler) CloseFactorySession(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.liveControl != nil {
		if s.guardSessionsRequestContext(w, r) {
			return
		}
		deletion := s.sessionDeletion
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

	s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
}

func (s *LifecycleHandler) writeDurableExecutionError(w http.ResponseWriter, err error) bool {
	if status, response, ok := factorysession.ExecutionErrorResponse(err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func durableSessionStartRequest(raw factorysessionexecution.StartRequest, synchronous bool, projectRoot string) (factorysessionexecution.SessionStartRequest, error) {
	if raw.EventConsumer != nil {
		return factorysessionexecution.SessionStartRequest{}, &factorysessionexecution.ValidationError{
			Field:   "eventConsumer",
			Message: "eventConsumer is not supported over HTTP",
		}
	}
	wait := factorysessionexecution.SessionOperationWait{}
	if raw.Wait != nil {
		if raw.Wait.TimeoutMillis != nil {
			wait.TimeoutMillis = *raw.Wait.TimeoutMillis
		}
		wait.CancelOnTimeout = raw.Wait.CancelOnTimeout
	}
	return factorysessionexecution.SessionStartRequest{
		Mode:           factorysessionexecution.SessionOperationModeDurable,
		Synchronous:    synchronous,
		Correlation:    factorysessionexecution.SessionOperationCorrelation{RequestID: raw.RequestID},
		Source:         raw.Source,
		Args:           raw.Args,
		Policy:         raw.RequestedPolicy,
		Orchestrator:   raw.Orchestrator,
		RuntimeOptions: raw.Runtime,
		Wait:           wait,
		FolderPath:     projectRoot,
	}, nil
}

func (s *LifecycleHandler) durableProjectRoot(ctx context.Context) (string, error) {
	defaultSession, err := s.sessionsRoot.Get(ctx, factorysessionexecution.SessionGetRequest{
		SessionID: factorysessionexecution.DefaultSessionID,
		Mode:      factorysessionexecution.SessionOperationModeLive,
	})
	if err != nil {
		if !errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
			return "", err
		}
		// A hosted process may select an explicit Factory Session ID. The
		// single live session is then the unambiguous Current Factory for
		// durable execution requests that have no session selector.
		listed, listErr := s.sessionsRoot.List(ctx, factorysessionexecution.SessionListRequest{
			Mode: factorysessionexecution.SessionOperationModeLive,
		})
		if listErr != nil {
			return "", listErr
		}
		if len(listed.Sessions) != 1 {
			if len(listed.Sessions) == 0 {
				return "", err
			}
			return "", &factorysessionexecution.ValidationError{
				Field: "projectRoot", Message: "current Factory Session is ambiguous",
			}
		}
		defaultSession.Session = listed.Sessions[0]
	}
	root := strings.TrimSpace(defaultSession.Session.FactoryDir)
	if root == "" {
		root = strings.TrimSpace(defaultSession.Session.FolderPath)
	}
	if root == "" {
		return "", &factorysessionexecution.ValidationError{Field: "projectRoot", Message: "current Factory project root is required"}
	}
	return root, nil
}

func (s *LifecycleHandler) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
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

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	projectRoot, err := s.durableProjectRoot(r.Context())
	if err != nil {
		s.writeSessionsRootErrorOrInternal(w, "", err, "current Factory is unavailable")
		return
	}
	mapped, err := durableSessionStartRequest(raw, false, projectRoot)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	started, err := s.sessionsRoot.Start(r.Context(), mapped)
	if err != nil {
		if s.writeSessionsRootError(w, "", err) {
			return
		}
		s.logger.Error("durable factory session async start failed", zap.Error(err))
		s.writeSessionsRootErrorOrInternal(w, "", err, "durable factory session execution failed")
		return
	}
	if started.Async == nil {
		s.logger.Error("canonical factory session start returned no async result")
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeCompatibilityWarning(w, "start_durable_factory_session_async", diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, factorysession.AsyncStartResponseToAPI(*started.Async))
	return

}

func (s *LifecycleHandler) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
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

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	projectRoot, err := s.durableProjectRoot(r.Context())
	if err != nil {
		s.writeSessionsRootErrorOrInternal(w, "", err, "current Factory is unavailable")
		return
	}
	mapped, err := durableSessionStartRequest(raw, true, projectRoot)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	started, err := s.sessionsRoot.Start(r.Context(), mapped)
	if err != nil {
		if s.writeSessionsRootError(w, "", err) {
			return
		}
		s.logger.Error("durable factory session sync start failed", zap.Error(err))
		s.writeSessionsRootErrorOrInternal(w, "", err, "durable factory session execution failed")
		return
	}
	if started.Sync == nil {
		s.logger.Error("canonical factory session start returned no sync result")
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeCompatibilityWarning(w, "start_durable_factory_session_sync", diagnostics.Paths())
	s.writeJSON(w, http.StatusOK, factorysession.SyncStartResponseToAPI(*started.Sync))
	return

}

type DurableExecutionSessionLister interface {
	ListSessions(
		context.Context,
		factorysessionexecution.ListSessionsRequest,
	) (factorysessionexecution.ListSessionsResult, error)
}

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func (s *ReadHandler) mergeScopedFactorySessionList(ctx context.Context, normalized factorysessionexecution.ListSessionsRequest) (factoryapi.ListFactorySessionsResponse, error) {
	result, err := mergeScopedSessionList(ctx, normalized, s.liveSessionLister, s.sessionsRoot)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ScopedSessionListResponseToAPI(result), nil
}

func (s *ReadHandler) writeDurableSessionReadError(w http.ResponseWriter, err error) bool {
	return s.writeSessionsRootError(w, "", err)
}

func (s *ReadHandler) writeDurableSessionListError(w http.ResponseWriter, err error) {
	if s.writeDurableSessionReadError(w, err) {
		return
	}
	s.logger.Error("list durable factory sessions failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
}
