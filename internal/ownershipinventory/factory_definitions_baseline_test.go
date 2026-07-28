package ownershipinventory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const packageTargetManifestRelativePath = "docs/internal/packaged-service-structure/package-target-manifest.json"

type packageTargetManifestRow struct {
	PackagePath string `json:"packagePath"`
	Disposition string `json:"disposition"`
	Destination string `json:"destination"`
}

type packageTargetManifestFile struct {
	Packages []packageTargetManifestRow `json:"packages"`
}

func TestFrozenInventoryFactoryDefinitionsRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, row := range inventory.Packages {
		if row.PackagePath == "pkg/services/factory_definitions" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if factoryDefinitionsCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "factory_definitions" {
			t.Fatalf("frozen inventory row retain→factory_definitions for %q", row.PackagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen inventory row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen inventory row %q missing successor/deletionCondition: %#v", row.PackagePath, row)
		}
	}
}

func TestFactoryDefinitionsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest, err := loadPackageTargetManifest(root)
	if err != nil {
		t.Fatalf("loadPackageTargetManifest() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, row := range manifest.Packages {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionMove {
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

func loadPackageTargetManifest(root string) (packageTargetManifestFile, error) {
	path := filepath.Join(root, filepath.FromSlash(packageTargetManifestRelativePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return packageTargetManifestFile{}, err
	}
	var manifest packageTargetManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return packageTargetManifestFile{}, err
	}
	return manifest, nil
}

func factoryDefinitionsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "internal":
		return true
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/catalog"):
		return true
	case strings.HasPrefix(rest, "internal/services/validation"):
		return true
	case strings.HasPrefix(rest, "internal/contracts"):
		return true
	case rest == "namevalue" || strings.HasPrefix(rest, "namevalue/"):
		return true
	default:
		return false
	}
}
