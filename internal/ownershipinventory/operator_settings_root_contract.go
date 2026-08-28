package ownershipinventory

import (
	"slices"
)

// OperatorSettingsThinRootContractFiles lists committed peer-facing root .go sources
// that remain at pkg/services/operator_settings/ during CLN-SET-CONTRACT-ROOTS.
// Confirmed against docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json
// (INV-SET-TOPLEVEL).
var OperatorSettingsThinRootContractFiles = []string{
	"acp_agent_profile.go",
	"acp_agent_profile_test.go",
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
	Cluster        string
	Files          []string
	Destination    string
	Classification string
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
	if err := validateOperatorSettingsRootGoInventory(inventory); err != nil {
		return err
	}
	return verifyOperatorSettingsRootGoProductionPolicy(inventory)
}

// VerifyOperatorSettingsRootReconciliation locks CLN-SET-CONTRACT-ROOTS story-001
// reconciliation: live root .go files, INV JSON inventory, Go mirror, and
// top-level directories agree before fold stories run.
//
// There is no longer a second ledger to reconcile against: open moves live in
// one consolidated ledger, so a manifest row and an inventory row cannot drift
// apart.
func VerifyOperatorSettingsRootReconciliation(root string) error {
	if err := VerifyOperatorSettingsRootGoInventory(root); err != nil {
		return err
	}
	if err := VerifyOperatorSettingsCommittedRootContractInventoryAlignment(root); err != nil {
		return err
	}
	if err := VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root); err != nil {
		return err
	}
	return nil
}
