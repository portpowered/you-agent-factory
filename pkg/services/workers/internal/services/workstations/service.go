// Package workstations defines the owner-private Workers workstation pool.
// The outer Workers service is the only consumer of this contract.
package workstations

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Route is one immutable workstation identity in a started pool snapshot.
type Route struct {
	WorkstationName string
	RunnerSelection workers.ResolvedRunnerSelection
	Executor        workers.WorkstationRequestExecutor
	Capacity        int
	QueueCapacity   int
}

// Service owns workstation pool lifecycle and route availability.
type Service interface {
	Start(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error)
	Stop(context.Context) (workers.WorkstationPoolStopResult, error)
	Route(context.Context, workers.WorkstationRouteRequest) (workers.WorkstationRouteResult, error)
	Dispatch(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}
