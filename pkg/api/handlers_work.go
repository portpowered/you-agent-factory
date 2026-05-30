package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

func (s *Server) SubmitWork(w http.ResponseWriter, r *http.Request) {
	req, err := decodeSubmitWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if req.WorkTypeName == "" {
		s.writeError(w, http.StatusBadRequest, "workTypeName is required", "BAD_REQUEST")
		return
	}

	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	content, err := submitWorkContent(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	submitReq := interfaces.SubmitRequest{
		Name:                   strings.TrimSpace(req.Name),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                factoryrequests.ResolveWorkRequestCurrentChainingTraceID(stringValue(req.CurrentChainingTraceId), stringValue(req.TraceId)),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	workRequest := factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

	result, err := s.runtime.SubmitWorkRequest(r.Context(), workRequest)
	if err != nil {
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("submit work failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, submitWorkResponseFromResult(result, factorysessions.DefaultSessionID))
}

func (s *Server) SubmitWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

	req, err := decodeSubmitWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if req.WorkTypeName == "" {
		s.writeError(w, http.StatusBadRequest, "workTypeName is required", "BAD_REQUEST")
		return
	}

	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	content, err := submitWorkContent(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	submitReq := interfaces.SubmitRequest{
		Name:                   strings.TrimSpace(req.Name),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                factoryrequests.ResolveWorkRequestCurrentChainingTraceID(stringValue(req.CurrentChainingTraceId), stringValue(req.TraceId)),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	workRequest := factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("submit work failed", zap.Error(err), zap.String("session_id", string(sessionID)))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, submitWorkResponseFromResult(result, string(sessionID)))
}

func (s *Server) UpsertWorkRequest(w http.ResponseWriter, r *http.Request, requestID string) {
	req, err := decodeWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if requestID == "" {
		s.writeError(w, http.StatusBadRequest, "request_id is required", "BAD_REQUEST")
		return
	}
	if req.RequestId == "" {
		s.writeError(w, http.StatusBadRequest, "requestId is required", "BAD_REQUEST")
		return
	}
	if req.RequestId != requestID {
		s.writeError(w, http.StatusBadRequest, "request_id path and requestId body must match", "BAD_REQUEST")
		return
	}

	workRequest, err := generatedWorkRequestToDomain(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	applyStableTraceToWorkRequest(&workRequest)
	result, err := s.runtime.SubmitWorkRequest(r.Context(), workRequest)
	if err != nil {
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		s.logger.Error("upsert work request failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work request", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, upsertWorkRequestResponse(result))
}

func (s *Server) UpsertWorkRequestBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, requestID string) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

	req, err := decodeWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if requestID == "" {
		s.writeError(w, http.StatusBadRequest, "request_id is required", "BAD_REQUEST")
		return
	}
	if req.RequestId == "" {
		s.writeError(w, http.StatusBadRequest, "requestId is required", "BAD_REQUEST")
		return
	}
	if req.RequestId != requestID {
		s.writeError(w, http.StatusBadRequest, "request_id path and requestId body must match", "BAD_REQUEST")
		return
	}

	workRequest, err := generatedWorkRequestToDomain(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	applyStableTraceToWorkRequest(&workRequest)
	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		s.logger.Error("upsert work request failed", zap.Error(err), zap.String("session_id", string(sessionID)))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work request", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, upsertWorkRequestResponse(result))
}

func upsertWorkRequestResponse(result interfaces.WorkRequestSubmitResult) factoryapi.UpsertWorkRequestResponse {
	works := make([]factoryapi.UpsertWorkRequestSubmittedWork, 0, len(result.Works))
	for _, work := range result.Works {
		works = append(works, factoryapi.UpsertWorkRequestSubmittedWork{
			Name:         work.Name,
			WorkTypeName: work.WorkTypeName,
			WorkId:       work.WorkID,
		})
	}
	return factoryapi.UpsertWorkRequestResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Works:     works,
	}
}

func applyStableTraceToWorkRequest(req *interfaces.WorkRequest) {
	if req == nil || len(req.Works) == 0 {
		return
	}
	traceID := ""
	if req.CurrentChainingTraceID != "" {
		traceID = req.CurrentChainingTraceID
	}
	if traceID == "" {
		for _, work := range req.Works {
			if work.CurrentChainingTraceID != "" {
				traceID = work.CurrentChainingTraceID
				break
			}
			if work.TraceID != "" {
				traceID = work.TraceID
				break
			}
		}
	}
	if traceID == "" {
		traceID = "trace-" + req.RequestID
	}
	if req.CurrentChainingTraceID == "" {
		req.CurrentChainingTraceID = traceID
	}
	for i := range req.Works {
		if req.Works[i].CurrentChainingTraceID == "" {
			if req.Works[i].TraceID != "" {
				req.Works[i].CurrentChainingTraceID = req.Works[i].TraceID
			} else {
				req.Works[i].CurrentChainingTraceID = traceID
			}
		}
		if req.Works[i].TraceID == "" {
			req.Works[i].TraceID = req.Works[i].CurrentChainingTraceID
		}
	}
}

func submitWorkBadRequestMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := err.Error()
	if strings.HasPrefix(message, "work_request:") {
		return submitWorkTypeNameMessage(message), true
	}
	if strings.Contains(message, "unknown work type") || strings.Contains(message, "work type") && strings.Contains(message, "not found") {
		return submitWorkTypeNameMessage(message), true
	}
	return "", false
}

func submitWorkTypeNameMessage(message string) string {
	message = strings.ReplaceAll(message, "work_type_name", "workTypeName")
	message = strings.ReplaceAll(message, "work_type_id", "workTypeName")
	if strings.Contains(message, "work type name") {
		return message
	}
	return strings.ReplaceAll(message, "work type", "work type name")
}

func (s *Server) MoveWork(w http.ResponseWriter, r *http.Request, id factoryapi.WorkOrTokenID) {
	workMover, ok := s.runtime.(factory.WorkMover)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "work move is unavailable", "INTERNAL_ERROR")
		return
	}
	s.handleMoveWork(
		w,
		r,
		string(id),
		func(ctx context.Context, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
			return workMover.MoveWork(ctx, workID, stateName, interfaces.WorkStateChangeSourceAPI, requestID)
		},
		func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
			return s.runtime.GetEngineStateSnapshot(ctx)
		},
	)
}

func (s *Server) MoveWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.handleMoveWork(
		w,
		r,
		string(id),
		func(ctx context.Context, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
			return sessionRuntime.MoveWorkForSession(ctx, string(sessionID), workID, stateName, requestID)
		},
		func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
			return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
		},
	)
}

type moveWorkInvoker func(ctx context.Context, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error)

type moveWorkSnapshotLoader func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)

func (s *Server) handleMoveWork(
	w http.ResponseWriter,
	r *http.Request,
	workID string,
	invoke moveWorkInvoker,
	loadSnapshot moveWorkSnapshotLoader,
) {
	req, err := decodeMoveWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	stateName := strings.TrimSpace(req.StateName)
	if stateName == "" {
		s.writeError(w, http.StatusBadRequest, "stateName is required", "BAD_REQUEST")
		return
	}

	requestID := strings.TrimSpace(stringValue(req.RequestId))
	if _, err := invoke(r.Context(), workID, stateName, requestID); err != nil {
		if status, message, code, ok := moveWorkHTTPError(err); ok {
			s.writeError(w, status, message, code)
			return
		}
		s.logger.Error("move work failed", zap.Error(err), zap.String("work_id", workID))
		s.writeError(w, http.StatusInternalServerError, "failed to move work", "INTERNAL_ERROR")
		return
	}

	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot after move failed", zap.Error(err), zap.String("work_id", workID))
		s.writeError(w, http.StatusInternalServerError, "failed to get work after move", "INTERNAL_ERROR")
		return
	}

	token, ok := findPublicWorkToken(snapshot.Marking.Tokens, workID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
		return
	}
	workNamesByID := publicWorkNamesByID(snapshot.Marking.Tokens)
	work := tokenToWork(token, snapshot.Topology)
	work.Relations = generatedWorkRelations(token, work.Name, workNamesByID)
	s.writeJSON(w, http.StatusOK, work)
}

func decodeMoveWorkRequestBody(body io.Reader) (factoryapi.MoveWorkRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	if len(payload) == 0 {
		return factoryapi.MoveWorkRequest{}, errors.New("request body is required")
	}
	var req factoryapi.MoveWorkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	return req, nil
}

func moveWorkHTTPError(err error) (status int, message, code string, ok bool) {
	switch {
	case errors.Is(err, engine.ErrMoveWorkNotFound):
		return http.StatusNotFound, "work not found", "NOT_FOUND", true
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return http.StatusNotFound, "factory session not found", "NOT_FOUND", true
	case errors.Is(err, engine.ErrMoveWorkInvalidState):
		return http.StatusBadRequest, "invalid target state for work type", "BAD_REQUEST", true
	case errors.Is(err, engine.ErrMoveWorkInFlightDispatch):
		return http.StatusBadRequest, "work is in an active dispatch", "BAD_REQUEST", true
	case errors.Is(err, engine.ErrMoveWorkEngineTerminated):
		return http.StatusBadRequest, "engine has terminated", "BAD_REQUEST", true
	case errors.Is(err, interfaces.ErrMoveWorkRequestAlreadyApplied):
		return http.StatusConflict, "Operator move request was already applied.", "MOVE_WORK_REQUEST_ALREADY_APPLIED", true
	default:
		return 0, "", "", false
	}
}
