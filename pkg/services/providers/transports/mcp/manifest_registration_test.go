package providersmcp_test

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	mcpAdapterPackagePath   = "pkg/services/providers/transports/mcp"
	mcpAdapterImportPath    = "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
	providersRootImportPath = "github.com/portpowered/infinite-you/pkg/services/providers"
	providersOwner          = "providers"
)

type coverageMinimumManifest struct {
	Lane     string `json:"lane"`
	Packages []struct {
		Package   string  `json:"package"`
		Minimum   float64 `json:"minimum"`
		Exception *struct {
			Kind string `json:"kind"`
		} `json:"exception"`
	} `json:"packages"`
}

func TestManifestRegistration_MCPAdapterPackageIsRegistered(t *testing.T) {
	t.Helper()

	assertPackageTargetManifestRegistration(t)
	assertOwnershipInventoryRegistration(t)
	assertCoverageMinimumRegistration(t, "unit", "docs/internal/baselines/go-unit-coverage-package-minimums.json")
	assertCoverageMinimumRegistration(t, "functional", "docs/internal/baselines/go-functional-coverage-package-minimums.json")
}

func TestProductionFiles_ImportOnlyProvidersRootOrStandardLibrary(t *testing.T) {
	t.Parallel()

	root := testutil.MustRepoPath(t, mcpAdapterPackagePath)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read MCP adapter directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Fatalf("decode import in %s: %v", path, err)
			}
			if importPath == providersRootImportPath || !strings.Contains(importPath, ".") {
				continue
			}
			t.Errorf("production MCP file %s imports non-owner package %q; only the Providers root is allowed", filepath.ToSlash(path), importPath)
		}
	}
}

func assertPackageTargetManifestRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/internal/packaged-service-structure/package-target-manifest.json"))
	if err != nil {
		t.Fatalf("read package-target manifest: %v", err)
	}

	var manifest struct {
		Inventory []string `json:"inventory"`
		Packages  []struct {
			PackagePath string `json:"packagePath"`
			Disposition string `json:"disposition"`
			Destination string `json:"destination"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode package-target manifest: %v", err)
	}

	foundInventory := false
	for _, packagePath := range manifest.Inventory {
		if packagePath == mcpAdapterPackagePath {
			foundInventory = true
			break
		}
	}
	if !foundInventory {
		t.Fatalf("package-target manifest inventory missing %q", mcpAdapterPackagePath)
	}

	for _, row := range manifest.Packages {
		if row.PackagePath != mcpAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("package-target manifest disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != providersOwner {
			t.Fatalf("package-target manifest destination = %q, want %q", row.Destination, providersOwner)
		}
		return
	}
	t.Fatalf("package-target manifest packages missing %q", mcpAdapterPackagePath)
}

func assertOwnershipInventoryRegistration(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, ownershipinventory.InventoryRelativePath))
	if err != nil {
		t.Fatalf("read ownership inventory: %v", err)
	}

	var inventory struct {
		Packages []ownershipinventory.PackageRow `json:"packages"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatalf("decode ownership inventory: %v", err)
	}

	for _, row := range inventory.Packages {
		if row.PackagePath != mcpAdapterPackagePath {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionRetain {
			t.Fatalf("ownership inventory disposition = %q, want %q", row.Disposition, ownershipinventory.DispositionRetain)
		}
		if row.Destination != providersOwner {
			t.Fatalf("ownership inventory destination = %q, want %q", row.Destination, providersOwner)
		}
		if row.DestinationKind != ownershipinventory.DestinationKindOwner {
			t.Fatalf(
				"ownership inventory destinationKind = %q, want %q",
				row.DestinationKind,
				ownershipinventory.DestinationKindOwner,
			)
		}
		return
	}
	t.Fatalf("ownership inventory packages missing %q", mcpAdapterPackagePath)
}

func assertCoverageMinimumRegistration(t *testing.T, lane string, relativePath string) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, relativePath))
	if err != nil {
		t.Fatalf("read %s coverage manifest: %v", lane, err)
	}

	var manifest coverageMinimumManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s coverage manifest: %v", lane, err)
	}
	if manifest.Lane != lane {
		t.Fatalf("%s coverage manifest lane = %q, want %q", relativePath, manifest.Lane, lane)
	}

	for _, entry := range manifest.Packages {
		if entry.Package != mcpAdapterImportPath {
			continue
		}
		if entry.Exception != nil {
			if entry.Exception.Kind != "measurement" {
				t.Fatalf("%s coverage exception kind for %q = %q, want measurement", lane, mcpAdapterImportPath, entry.Exception.Kind)
			}
			return
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, mcpAdapterImportPath)
		}
		return
	}
	t.Fatalf("%s coverage manifest missing %q", lane, mcpAdapterImportPath)
}
