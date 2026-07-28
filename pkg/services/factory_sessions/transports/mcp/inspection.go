package factorysession

import (
	"context"
	"errors"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// ListDispatchesInput is the MCP request shape for you.factory_session.list_dispatches.
type ListDispatchesInput struct {
	SessionID string `json:"sessionId"`
	Phase     string `json:"phase,omitempty"`
	Status    string `json:"status,omitempty"`
}

// ListDispatches returns deterministic dispatch summaries for one Factory Session
// through the you.factory_session.list_dispatches MCP tool.
func ListDispatches(
	ctx context.Context,
	service factorysessionexecution.ExecutionService,
	input ListDispatchesInput,
) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.ListFactorySessionDispatchesResponse](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.QueryDispatches(ctx, factorysessionexecution.DispatchQueryRequest{
		SessionID: sessionID,
		Filters: factorysessionexecution.DispatchFilters{
			Phase: input.Phase, Status: factorysessionexecution.DispatchStatus(input.Status),
		},
	})
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}
	mapped := apifactorysession.ListDispatchesResponseToAPI(result)
	return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Result: &mapped}
}

// ListArtifactsInput is the MCP request shape for you.factory_session.list_artifacts.
type ListArtifactsInput struct {
	SessionID string `json:"sessionId"`
}

// ListArtifacts returns deterministic FactoryArtifact summaries for one Factory
// Session through the you.factory_session.list_artifacts MCP tool.
func ListArtifacts(
	ctx context.Context,
	service factorysessionexecution.ExecutionService,
	input ListArtifactsInput,
) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.ListFactorySessionArtifactsResponse](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ListArtifacts(ctx, sessionID)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}

	mapped := apifactorysession.ListArtifactsResponseToAPI(result)
	return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Result: &mapped}
}

// ReadEventsInput is the MCP request shape for you.factory_session.read_events.
type ReadEventsInput struct {
	SessionID     string `json:"sessionId"`
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// ReadEventsResult is the MCP response shape for you.factory_session.read_events.
type ReadEventsResult struct {
	SessionID string                    `json:"sessionId"`
	Events    []factoryapi.FactoryEvent `json:"events,omitempty"`
}

// ReadEvents returns ordered Factory Session event facts for reconnect and
// inspection through the you.factory_session.read_events MCP tool.
func ReadEvents(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, input ReadEventsInput) ToolResponse[ReadEventsResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[ReadEventsResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	params := factoryapi.GetEventsBySessionIdParams{}
	if trimmed := input.AfterEventID; trimmed != "" {
		afterEventID := factoryapi.AfterEventId(trimmed)
		params.AfterEventId = &afterEventID
	}
	if input.AfterSequence != nil {
		sequence := factoryapi.AfterSequence(*input.AfterSequence)
		params.AfterSequence = &sequence
	}
	reconnect, err := apifactorysession.EventReconnectRequestFromAPI(params)
	if err == nil {
		reconnect, err = prepare.PrepareEventReconnect(reconnect)
	}
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		envelope := eventReadErrorEnvelope(sessionID, err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	mapped := ReadEventsResult{
		SessionID: result.SessionID,
		Events:    apifactorysession.EventReadResponseToAPI(result),
	}
	return ToolResponse[ReadEventsResult]{Result: &mapped}
}

// ControlInput is the MCP request shape for you.factory_session.control.
type ControlInput struct {
	SessionID         string                                        `json:"sessionId"`
	Operation         factoryapi.FactorySessionLifecycleControlKind `json:"operation"`
	RequestID         *string                                       `json:"requestId,omitempty"`
	Reason            *string                                       `json:"reason,omitempty"`
	DispatchID        *string                                       `json:"dispatchId,omitempty"`
	ApprovalPreviewID *string                                       `json:"approvalPreviewId,omitempty"`
	ApprovedPolicy    *map[string]any                               `json:"approvedPolicy,omitempty"`
}

// Control applies one durable Factory Session lifecycle control through the
// you.factory_session.control MCP tool.
func Control(
	ctx context.Context,
	service factorysessionexecution.ExecutionService,
	prepare RequestPreparation,
	input ControlInput,
) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionLifecycleControlResponse](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := invokeLifecycleControl(ctx, service, prepare, input)
	if err != nil {
		var controlErr *factorysessionexecution.ControlError
		if errors.As(err, &controlErr) {
			mapped := apifactorysession.ControlErrorToAPI(sessionID, controlErr)
			return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Result: &mapped}
		}
		envelope := controlErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}

	mapped := apifactorysession.LifecycleControlResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Result: &mapped}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this MCP control router keeps lifecycle kind dispatch on one seam.
func invokeLifecycleControl(
	ctx context.Context,
	service factorysessionexecution.ExecutionService,
	prepare RequestPreparation,
	input ControlInput,
) (factorysessionexecution.LifecycleControlResult, error) {
	sessionID := input.SessionID

	switch input.Operation {
	case factoryapi.FactorySessionLifecycleControlKindPause:
		control, err := prepareControlInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Pause(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindResume:
		control, err := prepareControlInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Resume(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindCancel:
		control, err := prepareControlInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Cancel(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindTerminate:
		control, err := prepareControlInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Terminate(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindApprove:
		approve, err := prepareApproveInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Approve(ctx, sessionID, approve)
	case factoryapi.FactorySessionLifecycleControlKindRetryDispatch:
		retry, err := prepareRetryDispatchInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.RetryDispatch(ctx, sessionID, retry)
	case factoryapi.FactorySessionLifecycleControlKindInterruptDispatch:
		interrupt, err := prepareInterruptDispatchInput(prepare, input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.InterruptDispatch(ctx, sessionID, interrupt)
	default:
		return factorysessionexecution.LifecycleControlResult{}, &factorysessionexecution.ExecutionValidationError{
			Field: "operation", Message: "unsupported lifecycle control operation",
		}
	}
}

func prepareControlInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.ControlRequest, error) {
	if prepare == nil {
		return factorysessionexecution.ControlRequest{}, errors.New("Factory Session request preparation is required")
	}
	return prepare.PrepareControl(factorysessionexecution.ControlRequest{
		RequestID: derefString(input.RequestID),
		Reason:    derefString(input.Reason),
	})
}

func prepareApproveInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.ApproveRequest, error) {
	if prepare == nil {
		return factorysessionexecution.ApproveRequest{}, errors.New("Factory Session request preparation is required")
	}
	approve := factorysessionexecution.ApproveRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		ApprovalPreviewID: derefString(input.ApprovalPreviewID),
	}
	if input.ApprovedPolicy != nil {
		approve.ApprovedPolicy = *input.ApprovedPolicy
	}
	return prepare.PrepareApprove(approve)
}

func prepareRetryDispatchInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.RetryDispatchRequest, error) {
	if prepare == nil {
		return factorysessionexecution.RetryDispatchRequest{}, errors.New("Factory Session request preparation is required")
	}
	retry := factorysessionexecution.RetryDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return prepare.PrepareRetryDispatch(retry)
}

func prepareInterruptDispatchInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.InterruptDispatchRequest, error) {
	if prepare == nil {
		return factorysessionexecution.InterruptDispatchRequest{}, errors.New("Factory Session request preparation is required")
	}
	interrupt := factorysessionexecution.InterruptDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return prepare.PrepareInterruptDispatch(interrupt)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
