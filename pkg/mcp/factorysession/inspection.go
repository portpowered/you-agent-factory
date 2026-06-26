package factorysession

import (
	"context"
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ListDispatchesInput is the MCP request shape for you.factory_session.list_dispatches.
type ListDispatchesInput struct {
	SessionID string `json:"sessionId"`
}

// ListDispatches returns deterministic dispatch summaries for one Factory Session
// through the you.factory_session.list_dispatches MCP tool.
func ListDispatches(
	service factorysessionexecution.Service,
	input ListDispatchesInput,
) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ListDispatches(context.Background(), sessionID)
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
	service factorysessionexecution.Service,
	input ListArtifactsInput,
) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ListArtifacts(context.Background(), sessionID)
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
func ReadEvents(service factorysessionexecution.Service, input ReadEventsInput) ToolResponse[ReadEventsResult] {
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
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ReadEvents(context.Background(), sessionID, reconnect)
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
	service factorysessionexecution.Service,
	input ControlInput,
) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := invokeLifecycleControl(service, input)
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
	service factorysessionexecution.Service,
	input ControlInput,
) (factorysessionexecution.LifecycleControlResult, error) {
	ctx := context.Background()
	sessionID := input.SessionID

	switch input.Operation {
	case factoryapi.FactorySessionLifecycleControlKindPause:
		control, err := normalizeControlInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Pause(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindResume:
		control, err := normalizeControlInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Resume(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindCancel:
		control, err := normalizeControlInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Cancel(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindTerminate:
		control, err := normalizeControlInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Terminate(ctx, sessionID, control)
	case factoryapi.FactorySessionLifecycleControlKindApprove:
		approve, err := normalizeApproveInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.Approve(ctx, sessionID, approve)
	case factoryapi.FactorySessionLifecycleControlKindRetryDispatch:
		retry, err := normalizeRetryDispatchInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.RetryDispatch(ctx, sessionID, retry)
	case factoryapi.FactorySessionLifecycleControlKindInterruptDispatch:
		interrupt, err := normalizeInterruptDispatchInput(input)
		if err != nil {
			return factorysessionexecution.LifecycleControlResult{}, err
		}
		return service.InterruptDispatch(ctx, sessionID, interrupt)
	default:
		return factorysessionexecution.LifecycleControlResult{}, factorysessionexecution.NewValidationError(
			"operation",
			"unsupported lifecycle control operation",
		)
	}
}

func normalizeControlInput(input ControlInput) (factorysessionexecution.ControlRequest, error) {
	return factorysessionexecution.NormalizeControlRequest(factorysessionexecution.ControlRequest{
		RequestID: derefString(input.RequestID),
		Reason:    derefString(input.Reason),
	})
}

func normalizeApproveInput(input ControlInput) (factorysessionexecution.ApproveRequest, error) {
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
	return factorysessionexecution.NormalizeApproveRequest(approve)
}

func normalizeRetryDispatchInput(input ControlInput) (factorysessionexecution.RetryDispatchRequest, error) {
	retry := factorysessionexecution.RetryDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return factorysessionexecution.NormalizeRetryDispatchRequest(retry)
}

func normalizeInterruptDispatchInput(input ControlInput) (factorysessionexecution.InterruptDispatchRequest, error) {
	interrupt := factorysessionexecution.InterruptDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return factorysessionexecution.NormalizeInterruptDispatchRequest(interrupt)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
