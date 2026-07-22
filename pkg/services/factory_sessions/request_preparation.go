package factorysessions

import execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"

// RequestPreparation is the exact Factory Sessions-owned operation used by
// transports to hand decoded external fields to the domain for canonical
// validation and normalization. Transports must not call execution helpers
// directly.
type RequestPreparation interface {
	PrepareStart(StartRequest) (StartRequest, error)
	PrepareControl(ControlRequest) (ControlRequest, error)
	PrepareApprove(ApproveRequest) (ApproveRequest, error)
	PrepareRetryDispatch(RetryDispatchRequest) (RetryDispatchRequest, error)
	PrepareInterruptDispatch(InterruptDispatchRequest) (InterruptDispatchRequest, error)
	PrepareListSessions(ListSessionsRequest) (ListSessionsRequest, error)
	PrepareResult(ResultRequest) (ResultRequest, error)
	PrepareEventReconnect(EventReconnectRequest) (EventReconnectRequest, error)
}

type requestPreparation struct{}

// NewRequestPreparation constructs the canonical Factory Sessions request
// preparation operation. Wire injects this one stateless role at every
// transport composition boundary.
func NewRequestPreparation() RequestPreparation { return requestPreparation{} }

func (requestPreparation) PrepareStart(request StartRequest) (StartRequest, error) {
	return execution.NormalizeStartRequest(request)
}

func (requestPreparation) PrepareControl(request ControlRequest) (ControlRequest, error) {
	return execution.NormalizeControlRequest(request)
}

func (requestPreparation) PrepareApprove(request ApproveRequest) (ApproveRequest, error) {
	return execution.NormalizeApproveRequest(request)
}

func (requestPreparation) PrepareRetryDispatch(request RetryDispatchRequest) (RetryDispatchRequest, error) {
	return execution.NormalizeRetryDispatchRequest(request)
}

func (requestPreparation) PrepareInterruptDispatch(request InterruptDispatchRequest) (InterruptDispatchRequest, error) {
	return execution.NormalizeInterruptDispatchRequest(request)
}

func (requestPreparation) PrepareListSessions(request ListSessionsRequest) (ListSessionsRequest, error) {
	return execution.NormalizeListSessionsRequest(request)
}

func (requestPreparation) PrepareResult(request ResultRequest) (ResultRequest, error) {
	return execution.NormalizeResultRequest(request)
}

func (requestPreparation) PrepareEventReconnect(request EventReconnectRequest) (EventReconnectRequest, error) {
	return execution.NormalizeEventReconnectRequest(request)
}
