package factorysessions

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
