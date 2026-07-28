package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// recordingsThinRootContractFiles lists committed peer-facing root .go sources
// that remain at pkg/services/recordings/ during CLN-REC-CONTRACT-ROOTS.
var recordingsThinRootContractFiles = []string{
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

type recordingsRootContractFoldTarget struct {
	cluster     string
	files       []string
	destination string
}

// recordingsExcessRootContractFolds mirrors internal/ownershipinventory
// RecordingsExcessRootContractFolds for package-target manifest checks.
var recordingsExcessRootContractFolds = []recordingsRootContractFoldTarget{
	{
		cluster: "artifacts",
		files: []string{
			"artifacts_import_boundary_test.go",
		},
		destination: "pkg/services/recordings/internal/services/artifacts_export",
	},
	{
		cluster: "replay",
		files: []string{
			"replay_contract.go",
			"replay_import_boundary_test.go",
		},
		destination: "pkg/services/recordings/internal/services/replay",
	},
	{
		cluster: "live_recording_target",
		files: []string{
			"live_recording_target.go",
			"live_recording_target_test.go",
		},
		destination: "pkg/services/recordings/internal/services/recording_lifecycle",
	},
}

func listRecordingsRootGoFiles(root string) ([]string, error) {
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

func recordingsRootContractInventory() []string {
	inventory := slices.Clone(recordingsThinRootContractFiles)
	for _, target := range recordingsExcessRootContractFolds {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyRecordingsRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(recordingsThinRootContractFiles, fileName) {
		return "thin_root_retain", "pkg/services/recordings", true
	}
	for _, target := range recordingsExcessRootContractFolds {
		if slices.Contains(target.files, fileName) {
			return "excess_fold", target.destination, true
		}
	}
	return "", "", false
}
