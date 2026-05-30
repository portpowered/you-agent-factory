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
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

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
