package factorysession

import (
	"context"
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
