package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const providersRootRelative = "pkg/services/providers"

// providersThinRootContractFiles mirrors ownershipinventory.ProvidersThinRootContractFiles.
var providersThinRootContractFiles = []string{
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
	"packaged_root_shape_test.go",
	"providers_root_contract_seal_test.go",
	"root_catalog_delegation_test.go",
	"root_contract_characterization_test.go",
	"root_wire_behavioral_boundary_test.go",
	"selection_characterization_test.go",
	"selection_contract.go",
	"service_contract.go",
}

type providersRootContractFoldTarget struct {
	cluster     string
	files       []string
	destination string
}

var providersExcessRootContractFolds = []providersRootContractFoldTarget{}

func listProvidersRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(providersRootRelative))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read Providers root: %w", err)
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

func providersRootContractInventory() []string {
	inventory := slices.Clone(providersThinRootContractFiles)
	for _, target := range providersExcessRootContractFolds {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyProvidersRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(providersThinRootContractFiles, fileName) {
		return "thin_root_retain", providersRootRelative, true
	}
	for _, target := range providersExcessRootContractFolds {
		if slices.Contains(target.files, fileName) {
			return "excess_fold", target.destination, true
		}
	}
	return "", "", false
}

func isProvidersPrivateRootContractFoldDestination(destination string) bool {
	if destination == providersRootRelative {
		return false
	}
	if destination == providersRootRelative+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, providersRootRelative+"/internal/")
}
