package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// BuildRuntime satisfies the published Workers root runtime-build slice.
// Nested IMP-WRK wiring that assembles concrete executors from this request
// remains out of scope for the CTR-WRK root-contract packet; peers and
// characterization fakes consume the root contracts directly.
func (s *Service) BuildRuntime(
	_ context.Context,
	_ workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	if s == nil {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: Worker execution service is required",
			workers.ErrIncompleteRuntimeAssembly,
		)
	}
	return workers.RuntimeBuildResult{}, fmt.Errorf(
		"%w: concrete Workers runtime-build assembly is not wired on the root yet",
		workers.ErrIncompleteRuntimeAssembly,
	)
}

// DispatchWorkstation satisfies the published Workers root workstation-dispatch
// slice. Nested IMP-WRK wiring that routes concrete workstation executors from
// this request remains out of scope for the CTR-WRK root-contract packet; peers
// and characterization fakes consume the root contracts directly.
func (s *Service) DispatchWorkstation(
	_ context.Context,
	_ workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if s == nil {
		return workers.WorkstationDispatchResult{}, fmt.Errorf(
			"%w: Worker execution service is required",
			workers.ErrIncompleteWorkstationDispatch,
		)
	}
	return workers.WorkstationDispatchResult{}, fmt.Errorf(
		"%w: concrete Workers workstation-dispatch is not wired on the root yet",
		workers.ErrIncompleteWorkstationDispatch,
	)
}
