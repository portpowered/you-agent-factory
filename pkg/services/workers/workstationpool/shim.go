// Package workstationpool is a transitional compile shim that re-exports
// workstation pool boundary contracts from the published Workers root. Canonical
// pool-boundary contracts live at
// pkg/services/workers/workstation_pool_boundary_contracts.go and under
// pkg/services/workers/internal/services/workstations/poolboundary; baseline
// deletion of this path is owned by DEL-WRK.
package workstationpool

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const DefaultRuntimePoolBindingCapacity = workers.DefaultRuntimePoolBindingCapacity

type (
	WorkstationExecutionService   = workers.WorkstationExecutionService
	WorkstationDispatchAcceptFunc = workers.WorkstationDispatchAcceptFunc
	WorkstationPoolBoundary       = workers.WorkstationPoolBoundary
	WorkstationPoolBoundaryConfig = workers.WorkstationPoolBoundaryConfig
)

var NewWorkstationPoolBoundary = workers.NewWorkstationPoolBoundary

// WorkstationExecutionServiceFromRoot adapts the published Workers root service
// to the pool-boundary execution port.
var WorkstationExecutionServiceFromRoot = workers.WorkstationExecutionServiceFromRoot
