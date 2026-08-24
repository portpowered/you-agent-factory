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
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

// Handler owns the generated HTTP operation implementations for Factory
// Sessions and their session-scoped Factory and Work resources.
type Adapter struct {
	sessionsRoot          factorysessions.Service
	liveControl           factorysessions.LiveControlService
	runtime               apisurface.RuntimeAPI
	factoryStatus         apisurface.FactoryStatusAPI
	sessions              apisurface.LiveSessionAPI
	invocation            apisurface.InvocationAPI
	factoryDefinitions    apisurface.FactorySaveAPI
	factoryValidation     factorydefinitions.SubmittedDefinitionValidationOperation
	workflowPreview       factoryruntime.WorkflowPreviewOperation
	durableExecution      apisurface.DurableSessionExecutionAPI
	durableLifecycle      apisurface.DurableSessionLifecycleAPI
	durableListing        apisurface.DurableSessionListingAPI
	durableResponseEvents DurableResponseEventsAPI
	durableLister         DurableExecutionSessionLister
	liveSessionLister     LiveSessionListReader
	workerPrompts         workers.PromptTemplates
	invocationWorkType    factorydefinitions.InvocationWorkTypeService
	sessionRequests       RequestPreparation
	logger                *zap.Logger
}

// Dependencies are the exact injected roles used by the Factory Sessions HTTP
// adapter. They are supplied by the already-opened runtime composition.
type Dependencies struct {
	SessionsRoot          factorysessions.Service
	LiveControl           factorysessions.LiveControlService
	Runtime               apisurface.RuntimeAPI
	FactoryStatus         apisurface.FactoryStatusAPI
	Sessions              apisurface.LiveSessionAPI
	Invocation            apisurface.InvocationAPI
	FactoryDefinitions    apisurface.FactorySaveAPI
	FactoryValidation     factorydefinitions.SubmittedDefinitionValidationOperation
	WorkflowPreview       factoryruntime.WorkflowPreviewOperation
	DurableExecution      apisurface.DurableSessionExecutionAPI
	DurableLifecycle      apisurface.DurableSessionLifecycleAPI
	DurableListing        apisurface.DurableSessionListingAPI
	DurableResponseEvents DurableResponseEventsAPI
	DurableLister         DurableExecutionSessionLister
	LiveSessionLister     LiveSessionListReader
	WorkerPrompts         workers.PromptTemplates
	InvocationWorkType    factorydefinitions.InvocationWorkTypeService
	SessionRequests       RequestPreparation
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

// DurableResponseEventsAPI is the only durable projection capability retained
// by the Sessions HTTP adapter. Canonical history, dispatch, and artifact
// reads are owned by Recordings; this narrow role exists solely for ephemeral
// FactoryResponseEvent delivery.
type DurableResponseEventsAPI interface {
	SubscribeDurableFactoryResponseEvents(
		context.Context,
		factorysessions.ResponseEventSubscriptionRequest,
	) (apisurface.FactoryResponseEventSubscription, error)
}

// NewHandler constructs an inert Factory Sessions HTTP adapter.
func NewHandler(deps Dependencies, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		sessionsRoot: deps.SessionsRoot, liveControl: deps.LiveControl,
		runtime: deps.Runtime, factoryStatus: deps.FactoryStatus,
		sessions:           deps.Sessions,
		invocation:         deps.Invocation,
		factoryDefinitions: deps.FactoryDefinitions, factoryValidation: deps.FactoryValidation,
		workflowPreview:  deps.WorkflowPreview,
		durableExecution: deps.DurableExecution, durableLifecycle: deps.DurableLifecycle,
		durableListing: deps.DurableListing, durableResponseEvents: deps.DurableResponseEvents,
		durableLister: deps.DurableLister, liveSessionLister: deps.LiveSessionLister,
		workerPrompts: deps.WorkerPrompts, invocationWorkType: deps.InvocationWorkType,
		sessionRequests: deps.SessionRequests,
		logger:          logger,
	}
}

// FactoryStatusSessionReader is the narrow session-scoped observation role
// used by the Factory Sessions status routes. Wire supplies the owner root
// only after confirming it exposes this capability.
type FactoryStatusSessionReader interface {
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

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if api == nil || api.sessions == nil || api.projector == nil {
		return factoryruntime.FactoryStatus{}, factoryruntime.ErrNotRunning
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	result, err := api.sessions.ObserveForSession(ctx, sessionID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return api.projector.ProjectFactoryStatusFromObservation(result.Observation), nil
}

// Server is retained as a private receiver alias while the moved handler files
// are kept mechanically identical to the established public behavior.
type Handler = Adapter
type Server = Adapter

// ListHumanApprovalsBySessionId returns the pending approvals projected from
// the selected live Factory Session's current session facts.
func (s *Adapter) ListHumanApprovalsBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListHumanApprovalsBySessionIdParams) {
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
func (s *Adapter) GetHumanApprovalBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, approvalID factoryapi.HumanApprovalID) {
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
