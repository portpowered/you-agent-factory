package ownershipinventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// PackageTargetManifestRelativePath is the committed package-target manifest
	// ledger paired with ownership-inventory for dual-ledger remap proof.
	PackageTargetManifestRelativePath = "docs/internal/packaged-service-structure/package-target-manifest.json"
)

// PackageTargetLedgerRow is one package row in the committed package-target manifest ledger.
type PackageTargetLedgerRow struct {
	PackagePath string `json:"packagePath"`
	Disposition string `json:"disposition"`
	Destination string `json:"destination"`
}

// PackageTargetLedger is the committed package-target manifest artifact.
type PackageTargetLedger struct {
	Packages []PackageTargetLedgerRow `json:"packages"`
}

// LoadPackageTargetLedger reads the committed package-target manifest from root.
func LoadPackageTargetLedger(root string) (PackageTargetLedger, error) {
	path := filepath.Join(root, filepath.FromSlash(PackageTargetManifestRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return PackageTargetLedger{}, fmt.Errorf("read package-target manifest %s: %w", PackageTargetManifestRelativePath, err)
	}
	var manifest PackageTargetLedger
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return PackageTargetLedger{}, fmt.Errorf("parse package-target manifest %s: %w", PackageTargetManifestRelativePath, err)
	}
	return manifest, nil
}

// VerifyProviderSessionsDualLedgerAlignment proves package-target-manifest and
// ownership-inventory agree on Provider Sessions package dispositions and that
// every move row's manifest destination aligns with the ownership successor path.
func VerifyProviderSessionsDualLedgerAlignment(root string) error {
	if err := VerifyProviderSessionsUnexpectedPublicSiblingRemaps(root); err != nil {
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

	const ownerPrefix = ProviderSessionsOwnerPackagePath + "/"
	for _, manifestRow := range manifest.Packages {
		if !isProviderSessionsOwnerPackagePath(manifestRow.PackagePath) {
			continue
		}
		ownershipRow, ok := ownershipByPath[manifestRow.PackagePath]
		if !ok {
			return fmt.Errorf("ownership inventory missing committed manifest row %q", manifestRow.PackagePath)
		}
		if err := validateProviderSessionsDualLedgerRow(manifestRow, ownershipRow); err != nil {
			return err
		}
	}

	for _, ownershipRow := range inventory.Packages {
		if !isProviderSessionsOwnerPackagePath(ownershipRow.PackagePath) {
			continue
		}
		if _, ok := manifestByPath[ownershipRow.PackagePath]; !ok {
			return fmt.Errorf("package-target manifest missing ownership inventory row %q", ownershipRow.PackagePath)
		}
	}

	return nil
}

func isProviderSessionsOwnerPackagePath(packagePath string) bool {
	return packagePath == ProviderSessionsOwnerPackagePath ||
		strings.HasPrefix(packagePath, ProviderSessionsOwnerPackagePath+"/")
}

func validateProviderSessionsDualLedgerRow(manifestRow PackageTargetLedgerRow, ownershipRow PackageRow) error {
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
