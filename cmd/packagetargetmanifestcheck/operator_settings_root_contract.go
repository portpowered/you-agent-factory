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
	"doc.go",
	"document_contract.go",
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
var operatorSettingsExcessRootContractFolds = []operatorSettingsRootContractFoldTarget{
	{
		cluster: "document_construction_bridge",
		files: []string{
			"document_bridge.go",
			"document_characterization_test.go",
			"document_owner_construct.go",
			"document_routing_test.go",
			"encode.go",
		},
		destination: "pkg/services/operator_settings/internal/services/document",
	},
	{
		cluster: "identity_input_index_inventory",
		files: []string{
			"identity.go",
			"identity_persist_test.go",
			"identity_test.go",
			"input_index.go",
			"input_index_load_cases.go",
			"input_index_parse_cases.go",
			"input_index_resolve_cases.go",
			"input_inventory_test.go",
			"inventory.go",
		},
		destination: "pkg/services/operator_settings/internal",
	},
	{
		cluster: "resolution_composition",
		files: []string{
			"environment_resolution.go",
			"provider_scope.go",
			"resolution_characterization_test.go",
			"resolution_composition.go",
		},
		destination: "pkg/services/operator_settings/internal/services/resolution",
	},
	{
		cluster: "providers_root_construction",
		files: []string{
			"providers_root_construct.go",
		},
		destination: "pkg/services/operator_settings/internal",
	},
	{
		cluster: "construction_ports",
		files: []string{
			"dependencies.go",
			"dependencies_test.go",
			"service_characterization_test.go",
			"testmain_test.go",
		},
		destination: "pkg/services/operator_settings/internal",
	},
	{
		cluster: "defaults_resolution_implementation",
		files: []string{
			"atomic_config_test.go",
			"operator_config.go",
			"operator_config_test.go",
		},
		destination: "pkg/services/operator_settings/internal/services/resolution",
	},
}

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
