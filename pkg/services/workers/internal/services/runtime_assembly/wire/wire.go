// Package wire constructs the private Workers Runtime Assembly subservice.
package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/internal/service"
)

// NewService constructs an inert runtime assembler from exact process-scoped
// Workers collaborators.
func NewService(
	runnerRegistry runners.Service,
	assembleBinding runtimeassembly.BindingAssembler,
) (runtimeassembly.Service, error) {
	if runnerRegistry == nil {
		return nil, fmt.Errorf("construct Workers Runtime Assembly: runner registry is required")
	}
	if assembleBinding == nil {
		return nil, fmt.Errorf("construct Workers Runtime Assembly: binding assembler is required")
	}
	return internalservice.New(runnerRegistry, assembleBinding), nil
}
