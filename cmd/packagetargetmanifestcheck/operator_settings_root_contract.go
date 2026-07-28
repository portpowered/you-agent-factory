package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const operatorSettingsRootRelative = "pkg/services/operator_settings"

// operatorSettingsThinRootContractFiles lists committed peer-facing root .go sources
// that remain at pkg/services/operator_settings/ during CLN-SET-CONTRACT-ROOTS.
// Mirrors internal/ownershipinventory OperatorSettingsThinRootContractFiles.
var operatorSettingsThinRootContractFiles = []string{
	"backend_scope.go",
	"config_document.go",
	"construction_ports_contract.go",
	"defaults_contract.go",
	"defaults_resolution.go",
	"doc.go",
	"document_contract.go",
	"input_inventory_contract.go",
	"resolution_contract.go",
	"service_contract.go",
	"root_contract_legacy_preservation_test.go",
	"service_root_contract_invariants_test.go",
}

type operatorSettingsRootContractFoldTarget struct {
	cluster     string
	files       []string
	destination string
}

// operatorSettingsExcessRootContractFolds mirrors internal/ownershipinventory
// OperatorSettingsExcessRootContractFolds for package-target manifest checks.
var operatorSettingsExcessRootContractFolds = []operatorSettingsRootContractFoldTarget{}

func listOperatorSettingsRootGoFiles(root string) ([]string, error) {
	settingsRoot := filepath.Join(root, filepath.FromSlash(operatorSettingsRootRelative))
	entries, err := os.ReadDir(settingsRoot)
	if err != nil {
		return nil, fmt.Errorf("read operator settings root: %w", err)
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

func operatorSettingsRootContractInventory() []string {
	inventory := slices.Clone(operatorSettingsThinRootContractFiles)
	for _, target := range operatorSettingsExcessRootContractFolds {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyOperatorSettingsRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(operatorSettingsThinRootContractFiles, fileName) {
		return "thin_root_retain", operatorSettingsRootRelative, true
	}
	for _, target := range operatorSettingsExcessRootContractFolds {
		if slices.Contains(target.files, fileName) {
			return "excess_fold", target.destination, true
		}
	}
	return "", "", false
}

func isOperatorSettingsPrivateRootContractFoldDestination(destination string) bool {
	if destination == operatorSettingsRootRelative {
		return false
	}
	if destination == operatorSettingsRootRelative+"/internal" {
		return true
	}
	return strings.HasPrefix(destination, operatorSettingsRootRelative+"/internal/")
}
