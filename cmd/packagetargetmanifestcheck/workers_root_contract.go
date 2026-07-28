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
	"execution_context.go",
	"execution_contracts.go",
	"execution_requests.go",
	"execution_tokens.go",
	"failure.go",
	"interfaces.go",
	"legacy_fold_boundary_test.go",
	"progress_observations.go",
	"provider_port.go",
	"provider_port_test.go",
	"response_drafts.go",
	"runtime_service.go",
	"safe_diagnostics.go",
	"sessions_consumer_boundary_test.go",
	"sessions_consumer_contracts.go",
	"service_import_boundary_test.go",
	"template_fields.go",
	"worker_vocabulary_boundary_test.go",
	"worker_vocabulary_contract.go",
	"workstation_result_contract_test.go",
}

type workersRootContractMoveTarget struct {
	cluster     string
	files       []string
	destination string
}

// workersRootContractMoveTargets mirrors internal/ownershipinventory
// WorkersRootContractMoveTargets for package-target manifest checks.
var workersRootContractMoveTargets = []workersRootContractMoveTarget{
	{
		cluster:     "runners",
		destination: "pkg/services/workers/internal/services/runners",
		files: []string{
			"opencode_agent_contract_test.go",
			"runner_policy.go",
			"runner_registry.go",
			"runner_registry_test.go",
		},
	},
	{
		cluster:     "workstations",
		destination: "pkg/services/workers/internal/services/workstations",
		files: []string{
			"env_diagnostics.go",
			"env_diagnostics_test.go",
			"inference_failure.go",
			"inference_failure_test.go",
			"invocation_executor_test.go",
			"model_invocation.go",
			"prompt_templates.go",
			"response_draft_validation.go",
			"template_fields_test.go",
			"token_lineage.go",
			"workstation_pool_boundary.go",
			"workstation_pool_boundary_test.go",
		},
	},
	{
		cluster:     "workers_internal",
		destination: "pkg/services/workers/internal",
		files: []string{
			"executor_test_helpers_test.go",
			"mock_workers.go",
			"mock_workers_config_test.go",
			"safe_diagnostics_codec.go",
		},
	},
}

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
