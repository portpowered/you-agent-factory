package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const workersRootRelative = "pkg/services/workers"

// WorkersThinRootContractFiles lists committed peer-facing root .go sources that
// remain at pkg/services/workers/ during CLN-WRK-CONTRACT-ROOTS.
// Confirmed against docs/internal/packaged-service-structure/workers-root-contract-surface-inventory.md
// (INV-WRK-TOPLEVEL).
var WorkersThinRootContractFiles = []string{
	"command.go",
	"execution_context.go",
	"execution_contracts.go",
	"execution_requests.go",
	"execution_requests_test.go",
	"execution_tokens.go",
	"failure.go",
	"interfaces.go",
	"legacy_fold_boundary_test.go",
	"progress_observations.go",
	"prompt_template_contracts.go",
	"provider_port.go",
	"provider_port_test.go",
	"response_drafts.go",
	"runtime_service.go",
	"runner_policy_contracts.go",
	"safe_diagnostics.go",
	"sessions_consumer_boundary_test.go",
	"sessions_consumer_contracts.go",
	"service_import_boundary_test.go",
	"template_fields.go",
	"template_fields_root_test.go",
	"validate_draft.go",
	"worker_vocabulary_boundary_test.go",
	"worker_vocabulary_contract.go",
	"workstation_contracts.go",
	"workstation_result_contract_test.go",
}

// WorkersRootContractMoveTarget names one root implementation cluster for
// CLN-WRK-CONTRACT-ROOTS without performing the move in INV-WRK-TOPLEVEL.
type WorkersRootContractMoveTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

// WorkersRootContractMoveTargets inventories root implementation clusters beyond
// the thin Workers service root contract.
var WorkersRootContractMoveTargets = []WorkersRootContractMoveTarget{
	{
		Cluster:     "workers_internal",
		Destination: workersPackagePrefix + "/internal",
		Files: []string{
			"executor_test_helpers_test.go",
			"mock_workers.go",
			"mock_workers_config_test.go",
			"safe_diagnostics_codec.go",
		},
	},
}

// ListWorkersRootGoFiles returns every live root-level .go file name under
// pkg/services/workers/.
func ListWorkersRootGoFiles(root string) ([]string, error) {
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

// ClassifyWorkersRootContractFile reports whether fileName is a committed
// thin-root keeper or an inventoried root-move target.
func ClassifyWorkersRootContractFile(fileName string) (kind string, moveTarget WorkersRootContractMoveTarget, ok bool) {
	if slices.Contains(WorkersThinRootContractFiles, fileName) {
		return "thin_root_retain", WorkersRootContractMoveTarget{}, true
	}
	for _, target := range WorkersRootContractMoveTargets {
		if slices.Contains(target.Files, fileName) {
			return "root_move", target, true
		}
	}
	return "", WorkersRootContractMoveTarget{}, false
}

// WorkersRootContractInventory returns the closed committed inventory of live root
// .go files: thin retain keepers plus root-move targets.
func WorkersRootContractInventory() []string {
	inventory := slices.Clone(WorkersThinRootContractFiles)
	for _, target := range WorkersRootContractMoveTargets {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// WorkersRootContractMoveCondition names the CLN packet that performs the move for
// one inventoried root cluster.
func WorkersRootContractMoveCondition(cluster string) string {
	return "CLN-WRK-CONTRACT-ROOTS cutover: move " + cluster + " root implementation cluster into private subservice"
}

// IsWorkersPrivateRootContractMoveDestination reports whether destination is a
// private move target under pkg/services/workers (never the owner root).
func IsWorkersPrivateRootContractMoveDestination(destination string) bool {
	if destination == workersPackagePrefix {
		return false
	}
	if destination == workersPackagePrefix+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, workersPackagePrefix+"/internal/")
}

// WorkersRootContractBaselineFileCount is the inventoried root .go file count
// before CLN-WRK-CONTRACT-ROOTS moves begin.
const WorkersRootContractBaselineFileCount = 31
