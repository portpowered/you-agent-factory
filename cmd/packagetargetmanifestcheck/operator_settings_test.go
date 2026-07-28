package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapCommittedOwnerPackageOperatorSettingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
	}{
		{
			path:        "pkg/services/operator_settings",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path:        "pkg/services/operator_settings/wire",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path:        "pkg/services/operator_settings/transports/http",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path: "pkg/services/operator_settings/internal/services/document/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/internal/services/document/wire",
				Disposition: DispositionRetain,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/identityinventory",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/identityinventory",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/identityinventory/input_index",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/identityinventory/input_index",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/servicewire",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/servicewire",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
		{
			path: "pkg/services/operator_settings/testlink",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/testlink",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
		{
			path: "pkg/services/operator_settings/testproviders",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/testproviders",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
		{
			path: "pkg/services/operator_settings/testdata/fixtures/valid",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/testdata/fixtures/valid",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
	}

	for _, tc := range cases {
		got, ok := mapCommittedOwnerPackage(tc.path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
		}
		if tc.wantRetain {
			if got.Disposition != DispositionRetain || got.Destination != tc.retainOwner {
				t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
			}
			continue
		}
		if got != tc.want {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
		}
	}
}

func TestOperatorSettingsTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["operator_settings"]
	for _, child := range spec.unexpected {
		rest := child
		destination, ok := nestedOwnerMoveDestination("operator_settings", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(operator_settings, %q) ok = false", rest)
		}
		if destination == "operator_settings" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}

func TestCommittedManifestOperatorSettingsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	manifestByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == DispositionRetain && row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row retain→operator_settings for %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsCommittedRootContractInventoryAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsCommittedRootContractInventoryAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsCommittedRootContractInventoryAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootReconciliationPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootReconciliation(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootReconciliation() error = %v", err)
	}
}

func TestOperatorSettingsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	ownership, err := ownershipinventory.Load(repoRoot)
	if err != nil {
		t.Fatalf("ownershipinventory.Load() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(ownership.Packages))
	for _, row := range ownership.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, row := range manifest.Packages {
		if row.PackagePath != "pkg/services/operator_settings" && !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != DispositionMove {
			continue
		}
		ownershipRow, ok := ownershipByPath[row.PackagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", row.PackagePath)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				row.PackagePath, row.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, packagePath := range manifest.Inventory {
		if packagePath == "pkg/services/operator_settings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if operatorSettingsCanonicalRetainRest(rest) {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRootForUnexpectedPublicSibling(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Destination == "operator_settings" {
			t.Fatalf("inventory path %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func operatorSettingsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/document"):
		return true
	case strings.HasPrefix(rest, "internal/services/resolution"):
		return true
	default:
		return false
	}
}
