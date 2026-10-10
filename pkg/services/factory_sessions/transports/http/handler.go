// Package http owns HTTP adaptation for Factory Session operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Sessions remain here with the owning service.
package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// Adapter composes completed Factory Sessions HTTP operation roles.
type Adapter struct {
	*LifecycleHandler
	*ReadHandler
	*AuthoringHandler
	*InvocationHandler
	*ObservationHandler
}

type Handler = Adapter

// NewHandler composes inert roles built by canonical Wire.
func NewHandler(lifecycle *LifecycleHandler, reads *ReadHandler, authoring *AuthoringHandler, invocation *InvocationHandler, observation *ObservationHandler) *Adapter {
	return &Adapter{lifecycle, reads, authoring, invocation, observation}
}

type RequestPreparation interface {
	PrepareStart(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	PrepareControl(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	PrepareApprove(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	PrepareRetryDispatch(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	PrepareListSessions(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	PrepareResult(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	PrepareEventReconnect(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

// FactoryStatusSessionReader is the narrow session-scoped observation role
// used by the Factory Sessions status routes. Wire supplies the owner root
// only after confirming it exposes this capability.
type FactoryStatusSessionReader interface {
	StartupRecoveryForSession(context.Context, string) (*factorysessions.StartupRecovery, error)
	ObserveForSession(
		context.Context,
		string,
		factoryruntime.ObserveRequest,
	) (factoryruntime.ObserveResult, error)
}

type factoryStatusAPI struct {
	sessions  FactoryStatusSessionReader
	projector factoryruntime.FactoryStatusProjector
}

// NewFactoryStatusAPI binds status projection to the Factory Sessions session
// router. Factory Sessions owns session identity; the selected session gateway
// owns the live observation.
func NewFactoryStatusAPI(
	sessions FactoryStatusSessionReader,
	projector factoryruntime.FactoryStatusProjector,
) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{sessions: sessions, projector: projector}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (apisurface.FactorySessionStatus, error) {
	if api == nil || api.sessions == nil || api.projector == nil {
		return apisurface.FactorySessionStatus{}, factoryruntime.ErrNotRunning
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	result, err := api.sessions.ObserveForSession(ctx, sessionID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return apisurface.FactorySessionStatus{}, err
	}
	recovery, err := api.sessions.StartupRecoveryForSession(ctx, sessionID)
	if err != nil {
		return apisurface.FactorySessionStatus{}, err
	}
	return apisurface.FactorySessionStatus{
		FactoryStatus:   api.projector.ProjectFactoryStatusFromObservation(result.Observation),
		StartupRecovery: recovery,
	}, nil
}

// ListHumanApprovalsBySessionId returns the pending approvals projected from
// the selected live Factory Session's current session facts.
func (s *LifecycleHandler) ListHumanApprovalsBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListHumanApprovalsBySessionIdParams) {
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
	s.writeJSON(w, http.StatusOK, factoryapi.ListHumanApprovalsResponse{Approvals: factorysession.HumanApprovalsToAPI(approvals)})
}

// GetHumanApprovalBySessionId returns one pending approval by its stable
// identity. Resolution is read-only; decision handling belongs to a later lane.
func (s *LifecycleHandler) GetHumanApprovalBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, approvalID factoryapi.HumanApprovalID) {
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

func (s *LifecycleHandler) pendingHumanApprovals(r *http.Request, sessionID string) ([]factorydefinitions.FactoryWorldHumanApproval, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("factory session id is required")
	}
	if s.liveControl != nil {
		projection, err := s.liveControl.ReadSessionDetail(r.Context(), sessionID)
		if err != nil {
			return nil, err
		}
		return append([]factorydefinitions.FactoryWorldHumanApproval(nil), projection.Runtime.PendingHumanApprovals...), nil
	}
	if s.sessionsRoot != nil {
		projection, err := s.sessionsRoot.ReadSessionDetail(r.Context(), sessionID)
		if err != nil {
			return nil, err
		}
		return append([]factorydefinitions.FactoryWorldHumanApproval(nil), projection.Runtime.PendingHumanApprovals...), nil
	}
	return nil, errors.New("factory session read service is unavailable")
}
