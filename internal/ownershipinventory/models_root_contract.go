package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const ModelsOwnerPackagePath = "pkg/services/models"

// ModelsThinRootContractFiles lists the committed peer-facing root .go
// sources that remain at pkg/services/models. The list is intentionally a
// closed inventory: later implementation-fold packets must either keep a
// file here deliberately or record it as an excess fold target.
var ModelsThinRootContractFiles = []string{
	"asset_scope_characterization_test.go",
	"assets_contract.go",
	"catalog_contract.go",
	"catalog_scope_characterization_test.go",
	"host_contract.go",
	"host_scope_characterization_test.go",
	"local_execution_contract.go",
	"managed_runtime_contract.go",
	"root_authority_seal_characterization_test.go",
	"root_slice_characterization_test.go",
	"runtime_config_contract.go",
	"runtime_construction_contract.go",
	"service_contract.go",
}

// ModelsRootContractFoldTarget names one excess root contract/helper cluster
// for a later Models contract-root fold. This packet has no such cluster: the
// existing root characterization tests and published vocabulary are all part
// of the currently committed root surface.
type ModelsRootContractFoldTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

var ModelsExcessRootContractFolds = []ModelsRootContractFoldTarget{}

// ListModelsRootGoFiles returns every live root-level .go file under Models,
// stable-sorted by file name.
func ListModelsRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(ModelsOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read Models root %s: %w", ModelsOwnerPackagePath, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		files = append(files, entry.Name())
	}
	slices.Sort(files)
	return files, nil
}

// ClassifyModelsRootContractFile reports whether fileName is a committed
// thin-root keeper or an inventoried excess fold target.
func ClassifyModelsRootContractFile(fileName string) (kind string, foldTarget ModelsRootContractFoldTarget, ok bool) {
	if slices.Contains(ModelsThinRootContractFiles, fileName) {
		return "thin_root_retain", ModelsRootContractFoldTarget{}, true
	}
	for _, target := range ModelsExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", ModelsRootContractFoldTarget{}, false
}

// ModelsRootContractInventory returns the closed committed inventory of live
// Models root .go files: thin retain keepers plus any future fold targets.
func ModelsRootContractInventory() []string {
	inventory := slices.Clone(ModelsThinRootContractFiles)
	for _, target := range ModelsExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// ModelsRootContractFoldCondition names the packet that performs the fold for
// one inventoried excess cluster.
func ModelsRootContractFoldCondition(cluster string) string {
	return "CLN-MODELS-CONTRACT-ROOTS cutover: fold excess " + cluster + " root contract cluster into private subservice"
}

// IsModelsPrivateRootContractFoldDestination reports whether destination is a
// private fold target under Models (never the owner root).
func IsModelsPrivateRootContractFoldDestination(destination string) bool {
	if destination == ModelsOwnerPackagePath {
		return false
	}
	if destination == ModelsOwnerPackagePath+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, ModelsOwnerPackagePath+"/internal/")
}

// VerifyModelsRootContractInventory proves the live Models root .go files
// match the committed root-contract inventory.
func VerifyModelsRootContractInventory(root string) error {
	live, err := ListModelsRootGoFiles(root)
	if err != nil {
		return err
	}
	want := ModelsRootContractInventory()
	if !slices.Equal(live, want) {
		return fmt.Errorf("Models root .go files drift: live=%v committed=%v", live, want)
	}
	for _, fileName := range live {
		if _, _, ok := ClassifyModelsRootContractFile(fileName); !ok {
			return fmt.Errorf("Models root .go file %q is not classified", fileName)
		}
	}
	return nil
}
