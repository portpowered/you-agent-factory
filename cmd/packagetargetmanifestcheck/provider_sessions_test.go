package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	providerSessionsServicePackagePath      = "pkg/services/provider_sessions/service"
	providerSessionsServiceManifestDest     = "provider_sessions/internal"
	providerSessionsServiceOwnershipSuccessor = "pkg/services/provider_sessions/internal"
)

func TestMapCommittedOwnerPackageProviderSessionsServiceMoveDestination(t *testing.T) {
	t.Parallel()

	got, ok := mapCommittedOwnerPackage(providerSessionsServicePackagePath)
	if !ok {
		t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", providerSessionsServicePackagePath)
	}
	want := PackageMapping{
		PackagePath: providerSessionsServicePackagePath,
		Disposition: DispositionMove,
		Destination: providerSessionsServiceManifestDest,
	}
	if got != want {
		t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", providerSessionsServicePackagePath, got, want)
	}
}

func TestMapCommittedOwnerPackageProviderSessionsPrivateReadersDoNotReplaceServiceMove(t *testing.T) {
	t.Parallel()

	privateReaders := []string{
		"pkg/services/provider_sessions/internal/services/codex_reader",
		"pkg/services/provider_sessions/internal/services/cursor_reader",
	}
	for _, packagePath := range privateReaders {
		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition != DispositionRetain {
			t.Fatalf("mapCommittedOwnerPackage(%q) disposition = %q, want retain", packagePath, got.Disposition)
		}
		if got.Destination != "provider_sessions/internal/services/codex_reader" &&
			got.Destination != "provider_sessions/internal/services/cursor_reader" {
			t.Fatalf("mapCommittedOwnerPackage(%q) destination = %q, want nested private reader destination", packagePath, got.Destination)
		}
	}

	serviceMapping, ok := mapCommittedOwnerPackage(providerSessionsServicePackagePath)
	if !ok {
		t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", providerSessionsServicePackagePath)
	}
	if serviceMapping.Disposition != DispositionMove || serviceMapping.Destination != providerSessionsServiceManifestDest {
		t.Fatalf("public service/ regressed after private readers exist: %#v", serviceMapping)
	}
}

func TestProviderSessionsCommittedManifestLocksServiceMoveDestination(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	var serviceRow *PackageMapping
	for index := range manifest.Packages {
		if manifest.Packages[index].PackagePath == providerSessionsServicePackagePath {
			serviceRow = &manifest.Packages[index]
			break
		}
	}
	if serviceRow == nil {
		t.Fatalf("committed package-target manifest missing %q", providerSessionsServicePackagePath)
	}
	if serviceRow.Disposition != DispositionMove {
		t.Fatalf("committed manifest row disposition = %q, want move", serviceRow.Disposition)
	}
	if serviceRow.Destination != providerSessionsServiceManifestDest {
		t.Fatalf("committed manifest row destination = %q, want %s", serviceRow.Destination, providerSessionsServiceManifestDest)
	}
}

func TestProviderSessionsDualLedgerAgreeOnServiceMoveDestination(t *testing.T) {
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

	manifestRow, ok := manifestPackageRow(manifest, providerSessionsServicePackagePath)
	if !ok {
		t.Fatalf("package-target manifest missing %q", providerSessionsServicePackagePath)
	}
	ownershipRow, ok := ownershipPackageRow(ownership, providerSessionsServicePackagePath)
	if !ok {
		t.Fatalf("ownership inventory missing %q", providerSessionsServicePackagePath)
	}
	if manifestRow.Disposition != DispositionMove || ownershipRow.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("service/ move disposition drift: manifest=%q ownership=%q", manifestRow.Disposition, ownershipRow.Disposition)
	}
	if manifestRow.Destination != providerSessionsServiceManifestDest {
		t.Fatalf("manifest destination = %q, want %s", manifestRow.Destination, providerSessionsServiceManifestDest)
	}
	if ownershipRow.Successor != providerSessionsServiceOwnershipSuccessor {
		t.Fatalf("ownership successor = %q, want %s", ownershipRow.Successor, providerSessionsServiceOwnershipSuccessor)
	}
}

func TestVerifyProviderSessionsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyProviderSessionsINVDispositionBeyondServicePassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
	}
}

func TestProviderSessionsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
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

	const ownerPrefix = "pkg/services/provider_sessions/"
	for _, row := range manifest.Packages {
		if row.PackagePath != "pkg/services/provider_sessions" && !strings.HasPrefix(row.PackagePath, ownerPrefix) {
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

func TestCommittedManifestProviderSessionsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	manifestByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == DispositionRetain && row.Destination == "provider_sessions" {
			t.Fatalf("committed manifest row retain→provider_sessions for %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination == "provider_sessions" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func TestProviderSessionsInventoryRejectsRetainToOwnerRootForUnexpectedPublicSibling(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	for _, child := range topLevel.Children {
		if child.Classification != ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling &&
			child.Classification != ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling {
			continue
		}
		packagePath := ownershipinventory.ProviderSessionsOwnerPackagePath + "/" + child.Directory
		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "provider_sessions" {
			t.Fatalf("unexpected retain→provider_sessions for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Destination == "provider_sessions" {
			t.Fatalf("inventory path %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func manifestPackageRow(manifest Manifest, packagePath string) (PackageMapping, bool) {
	for _, row := range manifest.Packages {
		if row.PackagePath == packagePath {
			return row, true
		}
	}
	return PackageMapping{}, false
}

func ownershipPackageRow(inventory ownershipinventory.Inventory, packagePath string) (ownershipinventory.PackageRow, bool) {
	for _, row := range inventory.Packages {
		if row.PackagePath == packagePath {
			return row, true
		}
	}
	return ownershipinventory.PackageRow{}, false
}
