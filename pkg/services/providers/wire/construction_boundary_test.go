package wire

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix              = "github.com/portpowered/infinite-you/"
	providersWirePackage      = modulePrefix + "pkg/services/providers/wire"
	catalogWireImport         = modulePrefix + "pkg/services/providers/internal/services/catalog/wire"
	executionWireImport       = modulePrefix + "pkg/services/providers/internal/services/execution/wire"
	rootServiceImport         = modulePrefix + "pkg/services/providers/internal/service"
	executionAdaptersPrefix   = modulePrefix + "pkg/services/providers/internal/services/execution/internal/adapters/"
	executionInternalService  = modulePrefix + "pkg/services/providers/internal/services/execution/internal/service"
)

// TestWireConstructionComposesCatalogAndExecutionOwners seals the sole
// service-local composition path: root wire must assemble Catalog and
// Execution through their owner wire packages and the published root facade.
func TestWireConstructionComposesCatalogAndExecutionOwners(t *testing.T) {
	t.Parallel()

	imports := listDirectImports(t, providersWirePackage)
	required := []string{
		catalogWireImport,
		executionWireImport,
		rootServiceImport,
	}
	for _, importPath := range required {
		if !containsImport(imports, importPath) {
			t.Fatalf(
				"%s must compose through %s; direct imports = %v",
				providersWirePackage,
				importPath,
				imports,
			)
		}
	}
}

// TestWireConstructionDoesNotImportExecutionAdaptersDirectly seals that root
// wire composes built-in adapters through execution/wire rather than reaching
// into adapter packages directly.
func TestWireConstructionDoesNotImportExecutionAdaptersDirectly(t *testing.T) {
	t.Parallel()

	for _, importPath := range listDirectImports(t, providersWirePackage) {
		if strings.HasPrefix(importPath, executionAdaptersPrefix) {
			t.Fatalf(
				"%s must not import execution adapter package %s directly",
				providersWirePackage,
				importPath,
			)
		}
	}
}

// TestWireConstructionDoesNotImportExecutionInternalServiceDirectly seals that
// root wire reaches execution through execution/wire rather than the private
// execution service package.
func TestWireConstructionDoesNotImportExecutionInternalServiceDirectly(t *testing.T) {
	t.Parallel()

	for _, importPath := range listDirectImports(t, providersWirePackage) {
		if importPath == executionInternalService ||
			strings.HasPrefix(importPath, executionInternalService+"/") {
			t.Fatalf(
				"%s must compose execution through execution/wire, not %s",
				providersWirePackage,
				importPath,
			)
		}
	}
}

func listDirectImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(strings.Trim(string(output), "[]"))
}

func containsImport(imports []string, importPath string) bool {
	for _, candidate := range imports {
		if candidate == importPath {
			return true
		}
	}
	return false
}
