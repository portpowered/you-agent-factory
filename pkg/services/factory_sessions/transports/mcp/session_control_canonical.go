package factorysession

import (
	"context"
	"errors"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func controlCanonical(ctx context.Context, sessions factorysessions.Service, prepare RequestPreparation, input ControlInput) ToolResponse[factoryapi.FactorySessionLifecycleControlResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionLifecycleControlResponse](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}
	request := factorysessions.SessionControlRequest{
		SessionID: input.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlOperation(input.Operation),
	}
	var err error
	switch request.Operation {
	case factorysessions.SessionControlPause, factorysessions.SessionControlResume, factorysessions.SessionControlCancel, factorysessions.SessionControlTerminate:
		request.Control, err = prepareControlInput(prepare, input)
	case factorysessions.SessionControlApprove:
		var approved factorysessions.ApproveRequest
		approved, err = prepareApproveInput(prepare, input)
		request.Approve = &approved
	case factorysessions.SessionControlRetryDispatch:
		var retry factorysessions.RetryDispatchRequest
		retry, err = prepareRetryDispatchInput(prepare, input)
		request.Retry = &retry
	case factorysessions.SessionControlInterruptDispatch:
		var interrupt factorysessions.InterruptDispatchRequest
		interrupt, err = prepareInterruptDispatchInput(prepare, input)
		request.Interrupt = &interrupt
	default:
		err = &factorysessions.ExecutionValidationError{Field: "operation", Message: "unsupported lifecycle control operation"}
	}
	if err != nil {
		envelope := controlErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}
	request.Correlation.RequestID = derefString(input.RequestID)
	result, err := sessions.Control(ctx, request)
	if err != nil {
		var controlErr *factorysessions.ControlError
		if errors.As(err, &controlErr) {
			mapped := apifactorysession.ControlErrorToAPI(input.SessionID, controlErr)
			return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Result: &mapped}
		}
		envelope := controlErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Error: &envelope}
	}
	mapped := apifactorysession.LifecycleControlResponseToAPI(factorysessions.LifecycleControlResult{
		SessionID: result.SessionID, Operation: factorysessions.LifecycleControlKind(result.Operation),
		Outcome: result.Outcome, Status: result.Status, Detail: result.Detail,
		ApprovalPreviewID: result.ApprovalPreviewID, DispatchID: result.DispatchID,
		RetryDispatchID: result.RetryDispatchID, Links: result.Links,
	})
	return ToolResponse[factoryapi.FactorySessionLifecycleControlResponse]{Result: &mapped}
}
