package service

import (
	"fmt"

	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewRoot constructs the inert Workers root from parent-private runtime
// assembly and workstation owners. It starts no lifecycle, runner execution, or
// workstation pool admission.
func NewRoot(
	runtimeAssembly runtimeassembly.Service,
	workstationsOwner workstations.Service,
) (workers.Service, error) {
	if runtimeAssembly == nil {
		return nil, fmt.Errorf("construct Workers: runtime assembly owner is required")
	}
	if workstationsOwner == nil {
		return nil, fmt.Errorf("construct Workers: workstations owner is required")
	}
	return &Service{
		runtimeAssembly: runtimeAssembly,
		workstations:    workstationsOwner,
	}, nil
}
