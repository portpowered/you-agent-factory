package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const workersRootRelative = "pkg/services/workers"

// workersThinRootContractFiles lists committed peer-facing root .go sources that
// remain at pkg/services/workers/ during CLN-WRK-CONTRACT-ROOTS.
// Mirrors internal/ownershipinventory WorkersThinRootContractFiles.
var workersThinRootContractFiles = []string{
	"command.go",
	"del_wrk_baseline_gate_test.go",
	"del_wrk_delete_ready_inventory_gate_test.go",
	"del_wrk_root_shape_test.go",
	"execution_context.go",
	"execution_contracts.go",
	"execution_requests.go",
	"execution_requests_test.go",
	"execution_tokens.go",
	"failure.go",
	"interfaces.go",
	"legacy_fold_boundary_test.go",
	"mock_workers_contracts.go",
	"mock_worker.go",
	"progress_observations.go",
	"prompt_template_contracts.go",
	"provider_port.go",
	"provider_port_test.go",
	"pty_contracts.go",
	"response_drafts.go",
	"runtime_service.go",
	"runner_policy_contracts.go",
	"runner_process.go",
	"runner_process_log_context.go",
	"safe_diagnostics.go",
	"safe_diagnostics_forward.go",
	"service_root_contract_seal_test.go",
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

type workersRootContractMoveTarget struct {
	cluster     string
	files       []string
	destination string
}

// workersRootContractMoveTargets mirrors internal/ownershipinventory
// WorkersRootContractMoveTargets for package-target manifest checks.
var workersRootContractMoveTargets = []workersRootContractMoveTarget{}

func listWorkersRootGoFiles(root string) ([]string, error) {
	workersRoot := filepath.Join(root, filepath.FromSlash(workersRootRelative))
	entries, err := os.ReadDir(workersRoot)
	if err != nil {
		return nil, fmt.Errorf("read workers root: %w", err)
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

func workersRootContractInventory() []string {
	inventory := slices.Clone(workersThinRootContractFiles)
	for _, target := range workersRootContractMoveTargets {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyWorkersRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(workersThinRootContractFiles, fileName) {
		return "thin_root_retain", "pkg/services/workers", true
	}
	for _, target := range workersRootContractMoveTargets {
		if slices.Contains(target.files, fileName) {
			return "root_move", target.destination, true
		}
	}
	return "", "", false
}

func isWorkersPrivateRootContractMoveDestination(destination string) bool {
	if destination == "pkg/services/workers" {
		return false
	}
	if destination == "pkg/services/workers/internal" {
		return true
	}
	return strings.HasPrefix(destination, "pkg/services/workers/internal/")
}
