package workers

import "context"

// WorkstationDispatchAdmissionFunc is retained as the internal handoff
// callback used by runtime-owned dispatch tests and adapters. It is not a
// Workers service capability; the canonical service exposes only Execute.
type WorkstationDispatchAdmissionFunc func()

// WorkstationDispatchAcceptFunc receives one detached dispatch result from a
// runtime-owned asynchronous dispatch operation.
type WorkstationDispatchAcceptFunc func(
	context.Context,
	WorkstationDispatchRequest,
	WorkstationDispatchResult,
	error,
)
