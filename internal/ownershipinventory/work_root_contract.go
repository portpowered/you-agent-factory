package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const workRootRelative = "pkg/services/work"

// WorkThinRootContractFiles lists committed peer-facing root .go sources that
// remain at pkg/services/work/ during CLN-WORK-CONTRACT-ROOTS.
// Confirmed against docs/internal/packaged-service-structure/work-root-contract-surface-inventory.md
// (INV-WORK-TOPLEVEL).
var WorkThinRootContractFiles = []string{
	"admission_contract.go",
	"content_contract.go",
	"content_materialization_public_seam_test.go",
	"content_materialize_contract.go",
	"content_staging_contract.go",
	"content_staging_public_seam_test.go",
	"contracts.go",
	"input.go",
	"input_test.go",
	"invocation_return_policy_contract.go",
	"invocation_return_policy_convert.go",
	"invocation_policy_service_test.go",
	"lineage_contract.go",
	"read_contract.go",
	"recordings_import_boundary_test.go",
	"recordings_request_boundary_test.go",
	"service_contract.go",
	"service_import_boundary_test.go",
	"service_peer_bindings.go",
	"service_peer_bindings_test.go",
	"service_root_contract_seal_test.go",
	"service_root_contract_test.go",
	"primary_result_test.go",
	"primary_result_regression_test.go",
	"wire_behavioral_proof_test.go",
	"legacy_packages_disposition_test.go",
}

// WorkRootContractFoldTarget names one excess root contract/helper cluster for
// CLN-WORK-CONTRACT-ROOTS without performing the fold in INV-WORK-TOPLEVEL.
type WorkRootContractFoldTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

// WorkExcessRootContractFolds inventories excess root contract/helper clusters
// beyond the thin Work service root contract.
var WorkExcessRootContractFolds = []WorkRootContractFoldTarget{}

// ListWorkRootGoFiles returns every live root-level .go file name under
// pkg/services/work/.
func ListWorkRootGoFiles(root string) ([]string, error) {
	workRoot := filepath.Join(root, filepath.FromSlash(workRootRelative))
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		return nil, fmt.Errorf("read work root: %w", err)
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

// ClassifyWorkRootContractFile reports whether fileName is a committed thin-root
// keeper or an inventoried excess fold target.
func ClassifyWorkRootContractFile(fileName string) (kind string, foldTarget WorkRootContractFoldTarget, ok bool) {
	if slices.Contains(WorkThinRootContractFiles, fileName) {
		return "thin_root_retain", WorkRootContractFoldTarget{}, true
	}
	for _, target := range WorkExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", WorkRootContractFoldTarget{}, false
}

// WorkRootContractInventory returns the closed committed inventory of live root
// .go files: thin retain keepers plus excess fold targets.
func WorkRootContractInventory() []string {
	inventory := slices.Clone(WorkThinRootContractFiles)
	for _, target := range WorkExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// WorkRootContractFoldCondition names the CLN packet that performs the fold for
// one inventoried excess cluster.
func WorkRootContractFoldCondition(cluster string) string {
	return "CLN-WORK-CONTRACT-ROOTS cutover: fold excess " + cluster + " root contract cluster into private subservice"
}

// IsWorkPrivateRootContractFoldDestination reports whether destination is a
// private fold target under pkg/services/work (never the owner root).
func IsWorkPrivateRootContractFoldDestination(destination string) bool {
	if destination == workPackagePrefix {
		return false
	}
	if destination == workPackagePrefix+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, workPackagePrefix+"/internal/")
}
