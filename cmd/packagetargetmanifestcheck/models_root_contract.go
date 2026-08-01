package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const modelsRootRelative = "pkg/services/models"

// modelsThinRootContractFiles mirrors ownershipinventory.ModelsThinRootContractFiles.
var modelsThinRootContractFiles = []string{
	"asset_scope_characterization_test.go",
	"assets_contract.go",
	"catalog_contract.go",
	"catalog_scope_characterization_test.go",
	"host_contract.go",
	"host_diagnostics_contract.go",
	"host_scope_characterization_test.go",
	"invocation_artifacts.go",
	"local_execution_contract.go",
	"managed_runtime_contract.go",
	"root_authority_seal_characterization_test.go",
	"root_slice_characterization_test.go",
	"runtime_config_contract.go",
	"runtime_construction_contract.go",
	"service_contract.go",
}

type modelsRootContractFoldTarget struct {
	cluster     string
	files       []string
	destination string
}

var modelsExcessRootContractFolds = []modelsRootContractFoldTarget{}

func listModelsRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(modelsRootRelative))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read Models root: %w", err)
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

func modelsRootContractInventory() []string {
	inventory := slices.Clone(modelsThinRootContractFiles)
	for _, target := range modelsExcessRootContractFolds {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyModelsRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(modelsThinRootContractFiles, fileName) {
		return "thin_root_retain", modelsRootRelative, true
	}
	for _, target := range modelsExcessRootContractFolds {
		if slices.Contains(target.files, fileName) {
			return "excess_fold", target.destination, true
		}
	}
	return "", "", false
}

func isModelsPrivateRootContractFoldDestination(destination string) bool {
	if destination == modelsRootRelative {
		return false
	}
	if destination == modelsRootRelative+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, modelsRootRelative+"/internal/")
}
