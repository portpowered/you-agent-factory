package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	transitionalValidationImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	transitionalNamevalueImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namevalue"
	transitionalTaxonomyImport     = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers/taxonomy"
	validationInternalsImport    = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
)

var forbiddenNonOwnerValidationImports = []struct {
	importPath string
	label      string
}{
	{validationInternalsImport, "validation internals"},
	{transitionalValidationImport, "transitional validation shim"},
	{transitionalNamevalueImport, "transitional namevalue shim"},
	{transitionalTaxonomyImport, "transitional workers/taxonomy shim"},
}

// TestNonOwnerProductionPackages_DoNotImportValidationInternalsOrTransitionalShims
// seals pss-cln-def-fold-validation story 004: production packages outside the
// Factory Definitions owner must reach validation through root contracts or
// owner wire construction, not validation internals or transitional shim paths.
func TestNonOwnerProductionPackages_DoNotImportValidationInternalsOrTransitionalShims(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}} {{join .Imports \" \"}}",
		"./...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		packagePath := fields[0]
		if isAllowedValidationOwnerImporter(packagePath) {
			continue
		}
		for _, importPath := range fields[1:] {
			for _, forbidden := range forbiddenNonOwnerValidationImports {
				if importPath != forbidden.importPath &&
					!strings.HasPrefix(importPath, forbidden.importPath+"/") {
					continue
				}
				t.Fatalf(
					"%s must not import %s %s; use %s or factory_definitions/wire",
					packagePath,
					forbidden.label,
					importPath,
					factoryDefinitionsOwnerPrefix,
				)
			}
		}
	}
}

func isAllowedValidationOwnerImporter(packagePath string) bool {
	return packagePath == factoryDefinitionsOwnerPrefix ||
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/")
}
