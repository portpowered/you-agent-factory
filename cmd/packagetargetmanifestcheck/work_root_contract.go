package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const workRootRelative = "pkg/services/work"

// workThinRootContractFiles lists committed peer-facing root .go sources that
// remain at pkg/services/work/ during CLN-WORK-CONTRACT-ROOTS.
// Mirrors internal/ownershipinventory WorkThinRootContractFiles.
var workThinRootContractFiles = []string{
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
	"read_contract.go",
	"recordings_import_boundary_test.go",
	"recordings_request_boundary_test.go",
	"service_contract.go",
	"service_import_boundary_test.go",
	"service_peer_bindings.go",
	"service_peer_bindings_test.go",
	"service_root_contract_seal_test.go",
	"service_root_contract_test.go",
	"wire_behavioral_proof_test.go",
	"legacy_packages_disposition_test.go",
}

type workRootContractFoldTarget struct {
	cluster     string
	files       []string
	destination string
}

// workExcessRootContractFolds mirrors internal/ownershipinventory
// WorkExcessRootContractFolds for package-target manifest checks.
var workExcessRootContractFolds = []workRootContractFoldTarget{
	{
		cluster: "request_admission",
		files: []string{
			"file_inputs.go",
			"file_inputs_test.go",
			"request_codec.go",
			"request_normalize.go",
			"request_normalize_test.go",
			"request_preparation.go",
			"request_preparation_test.go",
			"request_submit_test.go",
		},
		destination: "pkg/services/work/internal",
	},
	{
		cluster: "invocation_return_policy",
		files: []string{
			"arguments.go",
			"arguments_test.go",
			"invocation_input_preparation.go",
			"invocation_input_preparation_test.go",
			"invocation_policy_service.go",
			"invocation_policy_service_test.go",
			"primary_result.go",
			"primary_result_test.go",
			"primary_result_regression_test.go",
		},
		destination: "pkg/services/work/internal",
	},
	{
		cluster: "lineage_graph_modules",
		files: []string{
			"dependency_graph.go",
			"dependency_graph_test.go",
			"dependency_graph_markdown.go",
			"dependency_graph_markdown_test.go",
			"dependency_graph_mermaid.go",
			"dependency_graph_mermaid_test.go",
			"lineage.go",
			"visualization.go",
			"visualization_test.go",
		},
		destination: "pkg/services/work/internal/services/state_access",
	},
}

func listWorkRootGoFiles(root string) ([]string, error) {
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

func workRootContractInventory() []string {
	inventory := slices.Clone(workThinRootContractFiles)
	for _, target := range workExcessRootContractFolds {
		inventory = append(inventory, target.files...)
	}
	slices.Sort(inventory)
	return inventory
}

func classifyWorkRootContractFile(fileName string) (kind string, destination string, ok bool) {
	if slices.Contains(workThinRootContractFiles, fileName) {
		return "thin_root_retain", "pkg/services/work", true
	}
	for _, target := range workExcessRootContractFolds {
		if slices.Contains(target.files, fileName) {
			return "excess_fold", target.destination, true
		}
	}
	return "", "", false
}

func isWorkPrivateRootContractFoldDestination(destination string) bool {
	if destination == "pkg/services/work" {
		return false
	}
	if destination == "pkg/services/work/internal" {
		return true
	}
	return strings.HasPrefix(destination, "pkg/services/work/internal/")
}
