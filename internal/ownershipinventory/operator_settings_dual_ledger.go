package ownershipinventory

import (
	"fmt"
	"strings"
)

// VerifyOperatorSettingsDualLedgerAlignment proves package-target-manifest and
// ownership-inventory agree on Operator Settings package dispositions and that
// every move row's manifest destination aligns with the ownership successor path.
func VerifyOperatorSettingsDualLedgerAlignment(root string) error {
	if err := VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root); err != nil {
		return err
	}
	inventory, err := Load(root)
	if err != nil {
		return err
	}
	manifest, err := LoadPackageTargetLedger(root)
	if err != nil {
		return err
	}

	ownershipByPath := make(map[string]PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}
	manifestByPath := make(map[string]PackageTargetLedgerRow, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, manifestRow := range manifest.Packages {
		if !isOperatorSettingsOwnerPackagePath(manifestRow.PackagePath) {
			continue
		}
		ownershipRow, ok := ownershipByPath[manifestRow.PackagePath]
		if !ok {
			return fmt.Errorf("ownership inventory missing committed manifest row %q", manifestRow.PackagePath)
		}
		if err := validateOperatorSettingsDualLedgerRow(manifestRow, ownershipRow); err != nil {
			return err
		}
	}

	for _, ownershipRow := range inventory.Packages {
		if !isOperatorSettingsOwnerPackagePath(ownershipRow.PackagePath) {
			continue
		}
		if _, ok := manifestByPath[ownershipRow.PackagePath]; !ok {
			return fmt.Errorf("package-target manifest missing ownership inventory row %q", ownershipRow.PackagePath)
		}
	}

	return nil
}

func isOperatorSettingsOwnerPackagePath(packagePath string) bool {
	return packagePath == OperatorSettingsOwnerPackagePath ||
		strings.HasPrefix(packagePath, OperatorSettingsOwnerPackagePath+"/")
}

func validateOperatorSettingsDualLedgerRow(manifestRow PackageTargetLedgerRow, ownershipRow PackageRow) error {
	if manifestRow.Disposition != ownershipRow.Disposition {
		return fmt.Errorf("dual-ledger disposition drift for %q: manifest=%q ownership=%q",
			manifestRow.PackagePath, manifestRow.Disposition, ownershipRow.Disposition)
	}
	if manifestRow.Disposition != DispositionMove {
		return nil
	}
	wantSuccessor := "pkg/services/" + manifestRow.Destination
	if ownershipRow.Successor != wantSuccessor {
		return fmt.Errorf("dual-ledger move drift for %q: manifest destination %q => successor %q, ownership has %q",
			manifestRow.PackagePath, manifestRow.Destination, wantSuccessor, ownershipRow.Successor)
	}
	if strings.TrimSpace(ownershipRow.DeletionCondition) == "" {
		return fmt.Errorf("dual-ledger move row %q missing ownership deletionCondition", manifestRow.PackagePath)
	}
	return nil
}
