package ownershipinventory

import (
	"fmt"
	"slices"
	"strings"
)

// OperatorSettingsThinRootContractFiles lists committed peer-facing root .go sources
// that remain at pkg/services/operator_settings/ during CLN-SET-CONTRACT-ROOTS.
// Confirmed against docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json
// (INV-SET-TOPLEVEL).
var OperatorSettingsThinRootContractFiles = []string{
	"acp_integrations.go",
	"acp_integrations_test.go",
	"backend_scope.go",
	"config_document.go",
	"construction_ports_contract.go",
	"defaults_contract.go",
	"defaults_resolution.go",
	"doc.go",
	"document_contract.go",
	"input_inventory_contract.go",
	"resolution_contract.go",
	"root_wire_behavioral_boundary_test.go",
	"service_contract.go",
	"root_contract_legacy_preservation_test.go",
	"service_root_contract_invariants_test.go",
	"del_set_proof_gate_test.go",
	"packaged_root_shape_test.go",
}

// OperatorSettingsRootContractFoldTarget names one excess root contract/helper cluster
// for CLN-SET-CONTRACT-ROOTS without performing the fold in INV-SET-TOPLEVEL.
type OperatorSettingsRootContractFoldTarget struct {
	Cluster     string
	Files       []string
	Destination string
}

// OperatorSettingsExcessRootContractFolds inventories excess root contract/helper
// clusters beyond the thin Operator Settings service root contract.
var OperatorSettingsExcessRootContractFolds = []OperatorSettingsRootContractFoldTarget{}

// ClassifyOperatorSettingsRootContractFile reports whether fileName is a committed
// thin-root keeper or an inventoried excess fold target.
func ClassifyOperatorSettingsRootContractFile(fileName string) (kind string, foldTarget OperatorSettingsRootContractFoldTarget, ok bool) {
	if slices.Contains(OperatorSettingsThinRootContractFiles, fileName) {
		return "thin_root_retain", OperatorSettingsRootContractFoldTarget{}, true
	}
	for _, target := range OperatorSettingsExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", OperatorSettingsRootContractFoldTarget{}, false
}

// OperatorSettingsRootContractInventory returns the closed committed inventory of
// live root .go files: thin retain keepers plus excess fold targets.
func OperatorSettingsRootContractInventory() []string {
	inventory := slices.Clone(OperatorSettingsThinRootContractFiles)
	for _, target := range OperatorSettingsExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// IsOperatorSettingsPrivateRootContractFoldDestination reports whether destination
// is a private fold target under pkg/services/operator_settings (never the owner root).
func IsOperatorSettingsPrivateRootContractFoldDestination(destination string) bool {
	return isOperatorSettingsPrivateSuccessor(destination)
}

// VerifyOperatorSettingsCommittedRootContractInventoryAlignment proves the
// INV-SET-TOPLEVEL JSON inventory and the Go root-contract mirror classify every
// live root .go file the same way.
func VerifyOperatorSettingsCommittedRootContractInventoryAlignment(root string) error {
	inventory, err := LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		return err
	}
	wantInventory := OperatorSettingsRootContractInventory()
	committed := operatorSettingsRootGoFileNames(inventory.Files)
	if !slices.Equal(committed, wantInventory) {
		return fmt.Errorf("operator settings root contract inventory drift: json=%v go=%v", committed, wantInventory)
	}
	for _, file := range inventory.Files {
		kind, foldTarget, ok := ClassifyOperatorSettingsRootContractFile(file.File)
		if !ok {
			return fmt.Errorf("operator settings root go inventory file %q missing from Go root contract inventory", file.File)
		}
		switch file.Classification {
		case OperatorSettingsRootGoThinContract, OperatorSettingsRootGoThinContractTest:
			if kind != "thin_root_retain" {
				return fmt.Errorf("operator settings root go inventory file %q classification %q disagrees with Go kind %q", file.File, file.Classification, kind)
			}
			if strings.TrimSpace(file.FoldDestination) != "" || strings.TrimSpace(file.Cluster) != "" {
				return fmt.Errorf("operator settings thin root file %q must not set foldDestination/cluster in JSON inventory", file.File)
			}
		case OperatorSettingsRootGoFoldTargetConstruction,
			OperatorSettingsRootGoFoldTargetDocument,
			OperatorSettingsRootGoFoldTargetResolution,
			OperatorSettingsRootGoFoldTargetIdentity,
			OperatorSettingsRootGoFoldTargetProvidersConstruct,
			OperatorSettingsRootGoFoldTargetImplementation,
			OperatorSettingsRootGoFoldTargetImplTest:
			if kind != "excess_fold" {
				return fmt.Errorf("operator settings root go inventory fold target %q disagrees with Go kind %q", file.File, kind)
			}
			if file.FoldDestination != foldTarget.Destination {
				return fmt.Errorf("operator settings root go inventory file %q foldDestination = %q, Go fold target = %q", file.File, file.FoldDestination, foldTarget.Destination)
			}
			if file.Cluster != foldTarget.Cluster {
				return fmt.Errorf("operator settings root go inventory file %q cluster = %q, Go fold cluster = %q", file.File, file.Cluster, foldTarget.Cluster)
			}
		default:
			return fmt.Errorf("operator settings root go inventory file %q has unknown classification %q", file.File, file.Classification)
		}
	}
	return nil
}

// VerifyOperatorSettingsRootReconciliation locks CLN-SET-CONTRACT-ROOTS story-001
// reconciliation: live root .go files, INV JSON inventory, Go mirror, top-level
// directories, and ownership/package-target ledgers agree before fold stories run.
func VerifyOperatorSettingsRootReconciliation(root string) error {
	if err := VerifyOperatorSettingsRootGoInventory(root); err != nil {
		return err
	}
	if err := VerifyOperatorSettingsCommittedRootContractInventoryAlignment(root); err != nil {
		return err
	}
	if err := VerifyOperatorSettingsDualLedgerAlignment(root); err != nil {
		return err
	}
	return nil
}
