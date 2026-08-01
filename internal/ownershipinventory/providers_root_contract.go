package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const ProvidersOwnerPackagePath = "pkg/services/providers"

// ProvidersThinRootContractFiles lists every committed root-level Providers
// source. The process-edge registration vocabulary is owned by providers/wire;
// the Providers root retains only its published service vocabulary and proofs.
var ProvidersThinRootContractFiles = []string{
	"acp_configuration_contract.go",
	"acp_contract.go",
	"catalog_characterization_test.go",
	"catalog_contract.go",
	"doc.go",
	"execute_characterization_test.go",
	"execute_contract.go",
	"identity_characterization_test.go",
	"identity_contract.go",
	"lifecycle_contract.go",
	"root_catalog_delegation_test.go",
	"root_contract_characterization_test.go",
	"selection_characterization_test.go",
	"selection_contract.go",
	"service_contract.go",
}

type ProvidersRootContractFoldTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

var ProvidersExcessRootContractFolds = []ProvidersRootContractFoldTarget{}

func ListProvidersRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(ProvidersOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read Providers root %s: %w", ProvidersOwnerPackagePath, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		files = append(files, entry.Name())
	}
	slices.Sort(files)
	return files, nil
}

func ClassifyProvidersRootContractFile(fileName string) (kind string, foldTarget ProvidersRootContractFoldTarget, ok bool) {
	if slices.Contains(ProvidersThinRootContractFiles, fileName) {
		return "thin_root_retain", ProvidersRootContractFoldTarget{}, true
	}
	for _, target := range ProvidersExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", ProvidersRootContractFoldTarget{}, false
}

func ProvidersRootContractInventory() []string {
	inventory := slices.Clone(ProvidersThinRootContractFiles)
	for _, target := range ProvidersExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

func ProvidersRootContractFoldCondition(cluster string) string {
	return "CLN-PROV-CONTRACT-ROOTS cutover: fold excess " + cluster + " root contract cluster into private subservice"
}

func IsProvidersPrivateRootContractFoldDestination(destination string) bool {
	if destination == ProvidersOwnerPackagePath {
		return false
	}
	if destination == ProvidersOwnerPackagePath+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, ProvidersOwnerPackagePath+"/internal/")
}

func VerifyProvidersRootContractInventory(root string) error {
	live, err := ListProvidersRootGoFiles(root)
	if err != nil {
		return err
	}
	want := ProvidersRootContractInventory()
	if !slices.Equal(live, want) {
		return fmt.Errorf("Providers root .go files drift: live=%v committed=%v", live, want)
	}
	for _, fileName := range live {
		if _, _, ok := ClassifyProvidersRootContractFile(fileName); !ok {
			return fmt.Errorf("Providers root .go file %q is not classified", fileName)
		}
	}
	return nil
}
