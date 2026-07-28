package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestWorkersRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListWorkersRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListWorkersRootGoFiles() error = %v", err)
	}

	if len(live) != ownershipinventory.WorkersRootContractBaselineFileCount {
		t.Fatalf("live root .go file count = %d, want baseline %d", len(live), ownershipinventory.WorkersRootContractBaselineFileCount)
	}

	want := ownershipinventory.WorkersRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestWorkersThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"command.go",
		"execution_context.go",
		"execution_contracts.go",
		"execution_requests.go",
		"execution_requests_test.go",
		"execution_tokens.go",
		"failure.go",
		"interfaces.go",
		"legacy_fold_boundary_test.go",
		"mock_workers_contracts.go",
		"progress_observations.go",
		"prompt_template_contracts.go",
		"provider_port.go",
		"provider_port_test.go",
		"response_drafts.go",
		"runtime_service.go",
		"runner_policy_contracts.go",
		"safe_diagnostics.go",
		"safe_diagnostics_forward.go",
		"sessions_consumer_boundary_test.go",
		"sessions_consumer_contracts.go",
		"service_import_boundary_test.go",
		"template_fields.go",
		"template_fields_root_test.go",
		"validate_draft.go",
		"worker_vocabulary_boundary_test.go",
		"worker_vocabulary_contract.go",
		"workstation_contracts.go",
		"workstation_pool_boundary_contracts.go",
		"workstation_pool_boundary_impl.go",
		"workstation_pool_boundary_impl_test.go",
		"workstation_result_contract_test.go",
	}
	if !slices.Equal(ownershipinventory.WorkersThinRootContractFiles, want) {
		t.Fatalf("WorkersThinRootContractFiles = %v, want %v", ownershipinventory.WorkersThinRootContractFiles, want)
	}

	for _, fileName := range ownershipinventory.WorkersThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyWorkersRootContractFile(fileName)
		if !ok {
			t.Fatalf("ClassifyWorkersRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("ClassifyWorkersRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
	}
}

func TestWorkersRootContractMoveDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{}

	for _, target := range ownershipinventory.WorkersRootContractMoveTargets {
		wantDestination, ok := want[target.Cluster]
		if !ok {
			t.Fatalf("unexpected move cluster %q", target.Cluster)
		}
		if target.Destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.Cluster, target.Destination, wantDestination)
		}
		if len(target.Files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.Cluster)
		}
		if !ownershipinventory.IsWorkersPrivateRootContractMoveDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private move path under pkg/services/workers/internal", target.Cluster, target.Destination)
		}
		if !strings.HasPrefix(ownershipinventory.WorkersRootContractMoveCondition(target.Cluster), "CLN-WRK-CONTRACT-ROOTS") {
			t.Fatalf("move condition for %q missing CLN-WRK-CONTRACT-ROOTS prefix", target.Cluster)
		}
	}

	gotClusters := make([]string, 0, len(ownershipinventory.WorkersRootContractMoveTargets))
	for _, target := range ownershipinventory.WorkersRootContractMoveTargets {
		gotClusters = append(gotClusters, target.Cluster)
	}
	slices.Sort(gotClusters)
	wantClusters := []string{}
	slices.Sort(wantClusters)
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("move clusters = %v, want %v", gotClusters, wantClusters)
	}
}

func TestWorkersRootContractMoveDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/workers"
	for _, target := range ownershipinventory.WorkersRootContractMoveTargets {
		if target.Destination == ownerRoot {
			t.Fatalf("cluster %q moves to owner root retain destination", target.Cluster)
		}
		if !ownershipinventory.IsWorkersPrivateRootContractMoveDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private move path under %s/internal", target.Cluster, target.Destination, ownerRoot)
		}
		for _, fileName := range target.Files {
			kind, moveTarget, ok := ownershipinventory.ClassifyWorkersRootContractFile(fileName)
			if !ok {
				t.Fatalf("ClassifyWorkersRootContractFile(%q) ok = false", fileName)
			}
			if kind != "root_move" {
				t.Fatalf("ClassifyWorkersRootContractFile(%q) = %q, want root_move", fileName, kind)
			}
			if moveTarget.Destination == ownerRoot {
				t.Fatalf("root move file %q regressed to owner root retain destination", fileName)
			}
			if !ownershipinventory.IsWorkersPrivateRootContractMoveDestination(moveTarget.Destination) {
				t.Fatalf("root move file %q destination = %q, want private move path under %s/internal", fileName, moveTarget.Destination, ownerRoot)
			}
		}
	}
}

func TestWorkersRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListWorkersRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListWorkersRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, moveTarget, ok := ownershipinventory.ClassifyWorkersRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if moveTarget.Cluster != "" {
				t.Fatalf("thin root file %q carries move cluster %q", fileName, moveTarget.Cluster)
			}
		case "root_move":
			if moveTarget.Cluster == "" || moveTarget.Destination == "" {
				t.Fatalf("root move file %q missing cluster/destination: %#v", fileName, moveTarget)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}
