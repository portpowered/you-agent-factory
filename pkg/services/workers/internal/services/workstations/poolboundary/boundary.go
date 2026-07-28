// Package poolboundary is a transitional compile shim that re-exports workstation
// pool boundary contracts from the published Workers root. Canonical pool-boundary
// contracts and implementation live at
// pkg/services/workers/workstation_pool_boundary_contracts.go and
// pkg/services/workers/workstation_pool_boundary_impl.go; baseline deletion of
// this path is owned by DEL-WRK.
package poolboundary

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
