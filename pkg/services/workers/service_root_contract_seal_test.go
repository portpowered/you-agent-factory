package workers_test

import (
	"context"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestWorkersThinRootContractSetSealed locks pss-cln-wrk-contract-roots-006:
// CLN-WRK-CONTRACT-ROOTS cutover reduced the Workers root to the committed thin
// contract inventory with no remaining root-move targets.
func TestWorkersThinRootContractSetSealed(t *testing.T) {
	t.Parallel()

	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	live, err := ownershipinventory.ListWorkersRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListWorkersRootGoFiles() error = %v", err)
	}

	if len(live) != ownershipinventory.WorkersRootContractBaselineFileCount {
		t.Fatalf(
			"live root .go file count = %d, want post-cutover baseline %d",
			len(live),
			ownershipinventory.WorkersRootContractBaselineFileCount,
		)
	}
	if len(live) >= ownershipinventory.WorkersRootContractPreCutoverFileCount {
		t.Fatalf(
			"live root .go file count = %d, want materially fewer than pre-cutover %d",
			len(live),
			ownershipinventory.WorkersRootContractPreCutoverFileCount,
		)
	}

	want := ownershipinventory.WorkersRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}

	if len(ownershipinventory.WorkersRootContractMoveTargets) != 0 {
		t.Fatalf(
			"move targets remain = %v, want none at sealed root",
			ownershipinventory.WorkersRootContractMoveTargets,
		)
	}

	for _, folded := range []string{
		"runner_policy.go",
		"runner_registry.go",
		"mock_workers.go",
		"safe_diagnostics_codec.go",
		"model_invocation.go",
		"prompt_templates.go",
		"env_diagnostics.go",
		"inference_failure.go",
		"response_draft_validation.go",
		"token_lineage.go",
		"workstation_pool_boundary.go",
	} {
		if slices.Contains(live, folded) {
			t.Fatalf("%s still present at public Workers root after cutover", folded)
		}
	}

	var peerSurface workers.Service = &sealRootServiceFake{}
	if peerSurface == nil {
		t.Fatal("peer-shaped Service compile-time seal failed")
	}
}

// sealRootServiceFake is a minimal peer-shaped Workers root consumer used only
// for compile-time Service surface sealing in cutover delivery proofs.
type sealRootServiceFake struct{}

func (*sealRootServiceFake) InvokeModel(
	context.Context,
	string,
	models.Request,
) (models.Result, error) {
	return models.Result{}, nil
}

func (*sealRootServiceFake) BuildRuntime(
	context.Context,
	workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	return workers.RuntimeBuildResult{}, nil
}

func (*sealRootServiceFake) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (*sealRootServiceFake) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (*sealRootServiceFake) WorkstationRoute(
	context.Context,
	workers.WorkstationRouteRequest,
) (workers.WorkstationRouteResult, error) {
	return workers.WorkstationRouteResult{}, nil
}

func (*sealRootServiceFake) DispatchWorkstation(
	context.Context,
	workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return workers.WorkstationDispatchResult{}, nil
}

func (*sealRootServiceFake) CancelWorkstationDispatch(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}
