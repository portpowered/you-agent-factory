package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestRecordingsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListRecordingsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListRecordingsRootGoFiles() error = %v", err)
	}

	want := ownershipinventory.RecordingsRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestRecordingsThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"contracts.go",
		"contracts_test.go",
		"metadata.go",
		"runtime_import_boundary_test.go",
		"runtime_request_boundary_test.go",
		"service_import_boundary_test.go",
		"service_root_contract_fake_test.go",
		"service_root_contract_invariants_test.go",
		"service_root_contract_lifecycle_test.go",
		"service_root_contract_replay_test.go",
		"service_root_contract_seam_test.go",
		"wire_peer_import_boundary_test.go",
		"workers_root_boundary_test.go",
	}
	if !slices.Equal(ownershipinventory.RecordingsThinRootContractFiles, want) {
		t.Fatalf("RecordingsThinRootContractFiles = %v, want %v", ownershipinventory.RecordingsThinRootContractFiles, want)
	}

	for _, fileName := range ownershipinventory.RecordingsThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyRecordingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("ClassifyRecordingsRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("ClassifyRecordingsRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
	}
}

func TestRecordingsExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"artifacts":             "pkg/services/recordings/internal/services/artifacts_export",
		"live_recording_target": "pkg/services/recordings/internal/services/recording_lifecycle",
	}

	for _, target := range ownershipinventory.RecordingsExcessRootContractFolds {
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
		if !strings.HasPrefix(ownershipinventory.RecordingsRootContractFoldCondition(target.Cluster), "CLN-REC-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q missing CLN-REC-CONTRACT-ROOTS prefix", target.Cluster)
		}
	}

	gotClusters := make([]string, 0, len(ownershipinventory.RecordingsExcessRootContractFolds))
	for _, target := range ownershipinventory.RecordingsExcessRootContractFolds {
		gotClusters = append(gotClusters, target.Cluster)
	}
	slices.Sort(gotClusters)
	wantClusters := []string{
		"artifacts",
		"live_recording_target",
	}
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("fold clusters = %v, want %v", gotClusters, wantClusters)
	}
}

func TestRecordingsRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListRecordingsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListRecordingsRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, foldTarget, ok := ownershipinventory.ClassifyRecordingsRootContractFile(fileName)
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

func TestRecordingsExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/recordings"
	for _, target := range ownershipinventory.RecordingsExcessRootContractFolds {
		if target.Destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.Cluster)
		}
		if !strings.HasPrefix(target.Destination, ownerRoot+"/internal/") {
			t.Fatalf("cluster %q destination = %q, want private subservice path under %s/internal/", target.Cluster, target.Destination, ownerRoot)
		}
		for _, fileName := range target.Files {
			kind, foldTarget, ok := ownershipinventory.ClassifyRecordingsRootContractFile(fileName)
			if !ok {
				t.Fatalf("ClassifyRecordingsRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("ClassifyRecordingsRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if foldTarget.Destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
		}
	}
}
