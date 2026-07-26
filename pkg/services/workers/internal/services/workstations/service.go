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
}

// Service owns workstation pool lifecycle and route availability.
type Service interface {
	Start(context.Context, []Route) (workers.WorkstationPoolLifecycleOutcome, error)
	Stop(context.Context) (workers.WorkstationPoolLifecycleOutcome, error)
	Route(context.Context, string) error
}
