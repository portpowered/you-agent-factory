package service

import (
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewRoot constructs the inert Workers root from parent-private runtime
// assembly and workstation owners. It starts no lifecycle, runner execution, or
// workstation pool admission.
func NewRoot(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
) (workers.Service, error) {
	return workersinternal.NewRoot(runtimeAssembly, workstationsOwner)
}
