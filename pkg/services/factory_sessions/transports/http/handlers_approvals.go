package http

import (
	"errors"
	"net/http"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// ListHumanApprovalsBySessionId returns the pending approvals projected from
// the selected live Factory Session's canonical event history.
func (s *Adapter) ListHumanApprovalsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListHumanApprovalsBySessionIdParams,
) {
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	if params.Status != nil && string(*params.Status) != "PENDING" {
		s.writeError(w, http.StatusBadRequest, "unsupported human approval status; only PENDING is available", "BAD_REQUEST")
		return
	}
	approvals, err := s.pendingHumanApprovals(r, string(sessionID))
	if err != nil {
		if s.writeSessionsRootError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("list human approvals failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list human approvals", "INTERNAL_ERROR")
		return
	}
	mapped := factorysession.HumanApprovalsToAPI(approvals)
	s.writeJSON(w, http.StatusOK, factoryapi.ListHumanApprovalsResponse{Approvals: mapped})
}

// GetHumanApprovalBySessionId returns one pending approval by its stable
// identity. Resolution is read-only; decision handling belongs to a later
// lane.
func (s *Adapter) GetHumanApprovalBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	approvalID factoryapi.HumanApprovalID,
) {
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	approvals, err := s.pendingHumanApprovals(r, string(sessionID))
	if err != nil {
		if s.writeSessionsRootError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("get human approval failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get human approval", "INTERNAL_ERROR")
		return
	}
	for _, approval := range approvals {
		if approval.ApprovalID == string(approvalID) {
			s.writeJSON(w, http.StatusOK, factorysession.HumanApprovalToAPI(approval))
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "human approval not found", "NOT_FOUND")
}

func (s *Adapter) pendingHumanApprovals(r *http.Request, sessionID string) ([]factorydefinitions.FactoryWorldHumanApproval, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("factory session id is required")
	}
	if s.liveControl != nil {
		projection, err := s.liveControl.GetFactorySession(r.Context(), sessionID)
		if err != nil {
			return nil, err
		}
		return append([]factorydefinitions.FactoryWorldHumanApproval(nil), projection.Runtime.PendingHumanApprovals...), nil
	}
	if s.sessionsRoot != nil {
		projection, err := s.sessionsRoot.GetFactorySession(r.Context(), sessionID)
		if err != nil {
			return nil, err
		}
		return append([]factorydefinitions.FactoryWorldHumanApproval(nil), projection.Runtime.PendingHumanApprovals...), nil
	}
	return nil, errors.New("factory session read service is unavailable")
}
