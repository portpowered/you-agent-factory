package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
)

type service struct {
	runners         runners.Service
	assembleBinding runtimeassembly.BindingAssembler
}

var _ runtimeassembly.Service = (*service)(nil)

// New constructs an inert runtime assembler from process-scoped Workers
// collaborators.
func New(
	runnerRegistry runners.Service,
	assembleBinding runtimeassembly.BindingAssembler,
) runtimeassembly.Service {
	return &service{
		runners:         runnerRegistry,
		assembleBinding: assembleBinding,
	}
}

// Build assembles every requested role or returns an empty result with a typed
// Workers root error. No partial binding collection escapes on failure.
func (s *service) Build(
	ctx context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	snapshot, err := snapshotAndValidate(request)
	if err != nil {
		return workers.RuntimeBuildResult{}, err
	}
	if s == nil || s.runners == nil || s.assembleBinding == nil {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: runtime-assembly collaborators are required",
			workers.ErrIncompleteRuntimeAssembly,
		)
	}

	runnerBinding, err := s.runners.Resolve(runners.ResolutionRequest{
		Identity:             snapshot.RunnerID,
		RequiredCapabilities: snapshot.RequiredRunnerCapabilities,
	})
	if err != nil {
		return workers.RuntimeBuildResult{}, err
	}
	selection := workers.ResolvedRunnerSelection{
		RunnerID: runnerBinding.Identity,
		Source:   workers.RunnerSelectionSourceFactory,
	}
	if !completeSelection(selection) {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: runner %q resolved to an incomplete selection",
			workers.ErrIncompleteRuntimeAssembly,
			snapshot.RunnerID,
		)
	}

	bindings := make([]workers.AssembledRuntimeBinding, 0, len(snapshot.Roles))
	for _, role := range snapshot.Roles {
		binding, buildErr := s.assembleBinding(
			ctx,
			role,
			cloneOpeningOptions(snapshot.Opening),
			selection,
		)
		if buildErr != nil {
			return workers.RuntimeBuildResult{}, fmt.Errorf(
				"%w: assemble %s role %q: %w",
				workers.ErrRuntimeAssemblyRejected,
				role.Kind,
				role.Name,
				buildErr,
			)
		}
		if !completeBinding(binding) {
			return workers.RuntimeBuildResult{}, fmt.Errorf(
				"%w: %s role %q produced an incomplete binding",
				workers.ErrIncompleteRuntimeAssembly,
				role.Kind,
				role.Name,
			)
		}
		if binding.RoleName != role.Name ||
			binding.RoleKind != role.Kind ||
			binding.RunnerSelection != selection {
			return workers.RuntimeBuildResult{}, fmt.Errorf(
				"%w: %s role %q produced conflicting binding facts",
				workers.ErrRuntimeAssemblyRejected,
				role.Kind,
				role.Name,
			)
		}
		bindings = append(bindings, binding)
	}

	return workers.RuntimeBuildResult{
		RunnerSelection: selection,
		Bindings:        append([]workers.AssembledRuntimeBinding(nil), bindings...),
	}, nil
}

func completeSelection(selection workers.ResolvedRunnerSelection) bool {
	return selection.RunnerID != "" && selection.Source != ""
}

func completeBinding(binding workers.AssembledRuntimeBinding) bool {
	return binding.RoleName != "" &&
		(binding.RoleKind == workers.RuntimeBuildRoleKindWorker ||
			binding.RoleKind == workers.RuntimeBuildRoleKindWorkstation) &&
		completeSelection(binding.RunnerSelection)
}
