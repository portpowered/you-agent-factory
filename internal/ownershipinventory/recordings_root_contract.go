package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// RecordingsThinRootContractFiles lists committed peer-facing root .go sources
// that remain at pkg/services/recordings/ during CLN-REC-CONTRACT-ROOTS.
var RecordingsThinRootContractFiles = []string{
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
	"workers_root_boundary_test.go",
}

// RecordingsRootContractFoldTarget names one excess root contract/helper cluster
// for CLN-REC-CONTRACT-ROOTS without performing the fold in INV-REC-TOPLEVEL.
type RecordingsRootContractFoldTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

// RecordingsExcessRootContractFolds inventories excess root contract/helper
// clusters beyond the thin Recordings service root contract.
var RecordingsExcessRootContractFolds = []RecordingsRootContractFoldTarget{
	{
		Cluster: "event",
		Files: []string{
			"canonical_event_contract_test.go",
			"event_contract.go",
			"event_contract_test.go",
			"event_vocabulary_boundary_test.go",
			"events_import_boundary_test.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/canonical_ledger",
	},
	{
		Cluster: "world_state",
		Files: []string{
			"world_state_contract.go",
			"world_state_contract_test.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/projection_query",
	},
	{
		Cluster: "replay",
		Files: []string{
			"replay_contract.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/replay",
	},
	{
		Cluster: "dispatch",
		Files: []string{
			"dispatch_contract.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/projection_query",
	},
	{
		Cluster: "workstation_request",
		Files: []string{
			"workstation_requests.go",
			"workstation_requests_content_assert_test.go",
			"workstation_requests_test.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/projection_query",
	},
	{
		Cluster: "live_recording_target",
		Files: []string{
			"live_recording_target.go",
			"live_recording_target_test.go",
		},
		Destination: recordingsPackagePrefix + "/internal/services/recording_lifecycle",
	},
}

// ListRecordingsRootGoFiles returns every live root-level .go file name under
// pkg/services/recordings/.
func ListRecordingsRootGoFiles(root string) ([]string, error) {
	recordingsRoot := filepath.Join(root, filepath.FromSlash(recordingsRootRelative))
	entries, err := os.ReadDir(recordingsRoot)
	if err != nil {
		return nil, fmt.Errorf("read recordings root: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		files = append(files, name)
	}
	slices.Sort(files)
	return files, nil
}

// ClassifyRecordingsRootContractFile reports whether fileName is a committed
// thin-root keeper or an inventoried excess fold target.
func ClassifyRecordingsRootContractFile(fileName string) (kind string, foldTarget RecordingsRootContractFoldTarget, ok bool) {
	if slices.Contains(RecordingsThinRootContractFiles, fileName) {
		return "thin_root_retain", RecordingsRootContractFoldTarget{}, true
	}
	for _, target := range RecordingsExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", RecordingsRootContractFoldTarget{}, false
}

// RecordingsRootContractInventory returns the closed committed inventory of live
// root .go files: thin retain keepers plus excess fold targets.
func RecordingsRootContractInventory() []string {
	inventory := slices.Clone(RecordingsThinRootContractFiles)
	for _, target := range RecordingsExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// RecordingsRootContractFoldCondition names the CLN packet that performs the
// fold for one inventoried excess cluster.
func RecordingsRootContractFoldCondition(cluster string) string {
	return "CLN-REC-CONTRACT-ROOTS cutover: fold excess " + cluster + " root contract cluster into private subservice"
}
