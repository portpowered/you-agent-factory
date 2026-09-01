package apisurface

import (
	"context"
	"errors"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// TODO: these are mostly empty and should just be deprecated.
// FactorySaveAPI is the session-scoped factory definition read and persist seam.
type FactorySaveAPI interface {
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	SaveFactoryForSession(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
	SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error)
}

// RuntimeAPI owns the compatibility unscoped runtime reads retained by the
// HTTP routes. Work admission is owned by the Work transport and root.
type RuntimeAPI interface {
	SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error)
	GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error)
}

// FactoryStatusAPI is the exact detached Factory Runtime status read used by
// protocol transports. An empty session ID selects the default Factory Session
// runtime; a non-empty ID selects that Factory Session.
type FactoryStatusAPI interface {
	ProjectFactoryStatus(ctx context.Context, sessionID string) (factory.FactoryStatus, error)
}

// LiveSessionAPI owns live Factory Session inventory, lifecycle, and response
// event behavior without also aggregating work, invocation, models, or durable
// execution capabilities.
type LiveSessionAPI interface {
	ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(ctx context.Context, sessionID string, options interfaces.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error)
	OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(ctx context.Context, sessionID string) error
	PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	SubscribeFactoryResponseEventsForSession(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (FactoryResponseEventSubscription, error)
}

// WorkAPI is the session-scoped work submission, operator move, and runtime observability seam.
type WorkAPI interface {
	SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) error
}

// WorkReadAPI is the exact detached Work query and move-result role. It is
// separate from event ingress so transports cannot query engine snapshots.
type WorkReadAPI interface {
	ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error)
	GetWork(ctx context.Context, sessionID, id string) (work.ReadModel, error)
	MoveWorkAndRead(ctx context.Context, sessionID, id, stateName, requestID string) (work.ReadModel, error)
}

// InvocationAPI is the session-scoped factory invocation seam used by the API
// transport to submit one logical input and return the selected primary result.
type InvocationAPI interface {
	InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (FactoryInvocationResult, error)
}

// DurableSessionLifecycleAPI is the shared durable session read and lifecycle
// control seam for pause, resume, cancel, terminate, approve, and retry-dispatch.
type DurableSessionLifecycleAPI interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
	PauseDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ApproveDurableFactorySession(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	RetryDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	InterruptDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

// DurableSessionExecutionAPI is the shared durable factory-session execution start
// seam for async and sync dynamic workflow sessions. Live-session open remains on
// LiveSessionAPI and invocation remains on InvocationAPI.
type DurableSessionExecutionAPI interface {
	StartDurableFactorySessionAsync(ctx context.Context, request factorysessions.StartRequest) (factoryapi.FactorySessionExecutionResponse, error)
	StartDurableFactorySessionSync(ctx context.Context, request factorysessions.StartRequest) (factoryapi.FactorySessionSyncExecutionResponse, error)
}

// DurableSessionListingAPI is the shared scoped session listing seam for live and
// persisted durable execution sessions.
type DurableSessionListingAPI interface {
	ListDurableFactorySessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factoryapi.ListFactorySessionsResponse, error)
}

// DurableSessionProjectionAPI is the shared durable session result, dispatch,
// artifact, and event replay seam for dynamic workflow inspection.
type DurableSessionProjectionAPI interface {
	GetDurableFactorySessionResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factoryapi.FactorySessionResult, error)
	ListDurableFactorySessionDispatches(ctx context.Context, sessionID string, params factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error)
	ListDurableFactorySessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error)
	ReadDurableFactorySessionEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) error
	SubscribeDurableFactoryResponseEvents(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (FactoryResponseEventSubscription, error)
}

// FactoryInvocationResult carries the runtime-owned outcome of one session
// invocation request after input resolution and primary-result selection.
type FactoryInvocationResult = interfaces.FactoryInvocationResult

// InvocationResponseFromResult maps a shared invocation result onto the public
// invocation response contract used by both API and CLI JSON surfaces.
func InvocationResponseFromResult(result FactoryInvocationResult) factoryapi.InvocationResponse {
	response := factoryapi.InvocationResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Status:    factoryapi.InvocationTerminalStatus(result.Status),
	}
	if content := contentcontract.GeneratedPtrFromParts(result.PrimaryResult); content != nil {
		response.PrimaryResult = content
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		value := factoryapi.InvocationResponseErrorCode(code)
		response.ErrorCode = &value
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		response.Message = &message
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		response.SessionId = &sessionID
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		response.WorkId = &workID
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		response.WorkName = &workName
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		response.WorkState = &workState
	}
	if approvalID := strings.TrimSpace(result.ApprovalID); approvalID != "" {
		response.ApprovalId = &approvalID
	}
	if dispatchID := strings.TrimSpace(result.DispatchID); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if workstationID := strings.TrimSpace(result.WorkstationID); workstationID != "" {
		response.WorkstationId = &workstationID
	}
	if workstationName := strings.TrimSpace(result.WorkstationName); workstationName != "" {
		response.WorkstationName = &workstationName
	}
	if len(result.Decisions) > 0 {
		decisions := make([]factoryapi.InvocationResponseDecisions, 0, len(result.Decisions))
		for _, decision := range result.Decisions {
			decisions = append(decisions, factoryapi.InvocationResponseDecisions(decision))
		}
		response.Decisions = &decisions
	}
	return response
}

// FactoryInvocationResultFromResponse maps the public terminal response back
// to the shared CLI/runtime result used by transport-neutral presentation.
func FactoryInvocationResultFromResponse(response factoryapi.InvocationResponse) FactoryInvocationResult {
	result := FactoryInvocationResult{
		RequestID:     response.RequestId,
		TraceID:       response.TraceId,
		Status:        interfaces.InvocationTerminalStatus(response.Status),
		PrimaryResult: contentcontract.PartsFromGenerated(response.PrimaryResult),
	}
	if response.ErrorCode != nil {
		result.ErrorCode = string(*response.ErrorCode)
	}
	if response.Message != nil {
		result.Message = *response.Message
	}
	if response.SessionId != nil {
		result.SessionID = *response.SessionId
	}
	if response.WorkId != nil {
		result.WorkID = *response.WorkId
	}
	if response.WorkName != nil {
		result.WorkName = *response.WorkName
	}
	if response.WorkState != nil {
		result.WorkState = *response.WorkState
	}
	if response.ApprovalId != nil {
		result.ApprovalID = *response.ApprovalId
	}
	if response.DispatchId != nil {
		result.DispatchID = *response.DispatchId
	}
	if response.WorkstationId != nil {
		result.WorkstationID = *response.WorkstationId
	}
	if response.WorkstationName != nil {
		result.WorkstationName = *response.WorkstationName
	}
	if response.Decisions != nil {
		result.Decisions = make([]string, 0, len(*response.Decisions))
		for _, decision := range *response.Decisions {
			result.Decisions = append(result.Decisions, string(decision))
		}
	}
	return result
}

// RequestValidationError reports a stable client-side validation failure that
// should map to HTTP 400 at the transport boundary.
type RequestValidationError = interfaces.RequestValidationError

// ErrFactoryActivationRequiresIdle retains the public compatibility identity
// while activation policy is owned by the Factory domain.
var ErrFactoryActivationRequiresIdle = interfaces.ErrFactoryActivationRequiresIdle

// ErrInvalidNamedFactory retains the public compatibility identity while
// invalid persisted Factory definitions are classified by the config owner.
var ErrInvalidNamedFactory = interfaces.ErrInvalidNamedFactory

// ErrCurrentFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentFactoryNotFound = interfaces.ErrCurrentFactoryNotFound

// ErrFactorySessionNotFound reports that no live session matched the requested
// public session identifier.
var ErrFactorySessionNotFound = factorysessions.ErrSessionNotFound

// ErrInvalidEventReconnectCursor reports that the reconnect cursor did not
// match any recorded event in the targeted stream.
var ErrInvalidEventReconnectCursor = errors.New("invalid event reconnect cursor")

// ErrFactorySessionResultUnavailable reports that the requested session does not
// expose JavaScript result or partial-result reads.
var ErrFactorySessionResultUnavailable = errors.New("factory session result unavailable")

// ErrFactoryVersionStale retains the public compatibility error identity while
// Factory definition version policy is owned by the Factory domain.
var ErrFactoryVersionStale = interfaces.ErrFactoryVersionStale

// TopologyValidationError carries validation targets that the graph editor can
// map back to form fields, nodes, edges, or save-level messages.
type TopologyValidationError struct {
	Message string
	Targets []factoryapi.FactoryValidationTarget
}

func (e *TopologyValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "factory topology validation failed"
}

func (e *TopologyValidationError) Is(target error) bool {
	return target == ErrInvalidNamedFactory
}

func NewTopologyValidationError(message string, targets []factoryapi.FactoryValidationTarget) *TopologyValidationError {
	if message == "" {
		message = "factory topology validation failed"
	}
	return &TopologyValidationError{
		Message: message,
		Targets: append([]factoryapi.FactoryValidationTarget(nil), targets...),
	}
}

// DefaultCurrentFactoryName is the reserved current-factory identifier used
// when the active runtime is the root factory and no named-factory pointer
// exists.
const DefaultCurrentFactoryName factoryapi.FactoryName = "UNDEFINED"
