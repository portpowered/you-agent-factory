// Package requestpreparation owns transport-independent normalization of
// decoded Factory Session requests.
package requestpreparation

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

type service struct{}

// New constructs the canonical Factory Sessions request-preparation role.
func New() factorysessions.RequestPreparation { return service{} }

func (service) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return execution.NormalizeStartRequest(request)
}

func (service) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	return execution.NormalizeControlRequest(request)
}

func (service) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	return execution.NormalizeApproveRequest(request)
}

func (service) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	return execution.NormalizeRetryDispatchRequest(request)
}

func (service) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	return execution.NormalizeInterruptDispatchRequest(request)
}

func (service) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	return execution.NormalizeListSessionsRequest(request)
}

func (service) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	return execution.NormalizeResultRequest(request)
}

func (service) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	return execution.NormalizeEventReconnectRequest(request)
}
