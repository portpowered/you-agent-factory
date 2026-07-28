package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestWorkRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListWorkRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListWorkRootGoFiles() error = %v", err)
	}

	want := ownershipinventory.WorkRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestWorkThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"admission_contract.go",
		"content_contract.go",
		"content_materialization_public_seam_test.go",
		"content_materialize_contract.go",
		"content_staging_public_seam_test.go",
		"contracts.go",
		"input.go",
		"input_test.go",
		"invocation_return_policy_contract.go",
		"read_contract.go",
		"recordings_import_boundary_test.go",
		"recordings_request_boundary_test.go",
		"service_contract.go",
		"service_root_contract_seal_test.go",
		"service_root_contract_test.go",
		"service_import_boundary_test.go",
		"wire_behavioral_proof_test.go",
	}
	if !slices.Equal(ownershipinventory.WorkThinRootContractFiles, want) {
		t.Fatalf("WorkThinRootContractFiles = %v, want %v", ownershipinventory.WorkThinRootContractFiles, want)
	}

	for _, fileName := range ownershipinventory.WorkThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyWorkRootContractFile(fileName)
		if !ok {
			t.Fatalf("ClassifyWorkRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("ClassifyWorkRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
	}
}

func TestWorkExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"request_admission":            "pkg/services/work/internal",
		"invocation_return_policy":     "pkg/services/work/internal",
		"service_peer_bindings":        "pkg/services/work/internal",
		"lineage_graph_modules":        "pkg/services/work/internal/services/state_access",
		"state_access_query":           "pkg/services/work/internal/services/state_access",
		"content_staging_impl":         "pkg/services/work/internal/services/content_staging",
		"content_materialization_impl": "pkg/services/work/internal/services/content_materialization",
	}

	for _, target := range ownershipinventory.WorkExcessRootContractFolds {
		wantDestination, ok := want[target.Cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", target.Cluster)
		}
		if target.Destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.Cluster, target.Destination, wantDestination)
		}
		if len(target.Files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.Cluster)
		}
		if !ownershipinventory.IsWorkPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under pkg/services/work/internal", target.Cluster, target.Destination)
		}
		if !strings.HasPrefix(ownershipinventory.WorkRootContractFoldCondition(target.Cluster), "CLN-WORK-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q missing CLN-WORK-CONTRACT-ROOTS prefix", target.Cluster)
		}
	}

	gotClusters := make([]string, 0, len(ownershipinventory.WorkExcessRootContractFolds))
	for _, target := range ownershipinventory.WorkExcessRootContractFolds {
		gotClusters = append(gotClusters, target.Cluster)
	}
	slices.Sort(gotClusters)
	wantClusters := []string{
		"content_materialization_impl",
		"content_staging_impl",
		"invocation_return_policy",
		"lineage_graph_modules",
		"request_admission",
		"service_peer_bindings",
		"state_access_query",
	}
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("fold clusters = %v, want %v", gotClusters, wantClusters)
	}
}

func TestWorkExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/work"
	for _, target := range ownershipinventory.WorkExcessRootContractFolds {
		if target.Destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.Cluster)
		}
		if !ownershipinventory.IsWorkPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under %s/internal", target.Cluster, target.Destination, ownerRoot)
		}
		for _, fileName := range target.Files {
			kind, foldTarget, ok := ownershipinventory.ClassifyWorkRootContractFile(fileName)
			if !ok {
				t.Fatalf("ClassifyWorkRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("ClassifyWorkRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if foldTarget.Destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
			if !ownershipinventory.IsWorkPrivateRootContractFoldDestination(foldTarget.Destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under %s/internal", fileName, foldTarget.Destination, ownerRoot)
			}
		}
	}
}

func TestWorkRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListWorkRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListWorkRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, foldTarget, ok := ownershipinventory.ClassifyWorkRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if foldTarget.Cluster != "" {
				t.Fatalf("thin root file %q carries fold cluster %q", fileName, foldTarget.Cluster)
			}
		case "excess_fold":
			if foldTarget.Cluster == "" || foldTarget.Destination == "" {
				t.Fatalf("excess fold file %q missing cluster/destination: %#v", fileName, foldTarget)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}
