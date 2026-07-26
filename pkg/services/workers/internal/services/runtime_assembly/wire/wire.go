// Package wire constructs the private Workers Runtime Assembly subservice.
package wire

import (
	"fmt"

	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/internal/service"
)

// NewService constructs an inert runtime assembler from exact process-scoped
// Workers collaborators.
func NewService(
	resolveRunner runtimeassembly.RunnerResolver,
	assembleBinding runtimeassembly.BindingAssembler,
) (runtimeassembly.Service, error) {
	if resolveRunner == nil {
		return nil, fmt.Errorf("construct Workers Runtime Assembly: runner resolver is required")
	}
	if assembleBinding == nil {
		return nil, fmt.Errorf("construct Workers Runtime Assembly: binding assembler is required")
	}
	return internalservice.New(resolveRunner, assembleBinding), nil
}
