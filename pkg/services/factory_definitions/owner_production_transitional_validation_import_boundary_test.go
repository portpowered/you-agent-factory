package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

var forbiddenOwnerTransitionalValidationImports = []struct {
	importPath string
	label      string
}{
	{transitionalValidationImport, "transitional validation shim"},
	{transitionalNamevalueImport, "transitional namevalue shim"},
	{transitionalTaxonomyImport, "transitional workers/taxonomy shim"},
}

var forbiddenOwnerValidationInternalsImport = validationInternalsImport

// TestOwnerProductionPackages_DoNotImportTransitionalValidationShims seals
// pss-cln-def-fold-validation story 004: owner production packages must not
// treat public validation/, namevalue, or workers/taxonomy shims as the
// implementation home after cutover.
func TestOwnerProductionPackages_DoNotImportTransitionalValidationShims(t *testing.T) {
	t.Parallel()

	for _, line := range listOwnerProductionPackageImports(t) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		packagePath := fields[0]
		if isAllowedOwnerTransitionalValidationImporter(packagePath) {
			continue
		}
		for _, importPath := range fields[1:] {
			for _, forbidden := range forbiddenOwnerTransitionalValidationImports {
				if importPath != forbidden.importPath &&
					!strings.HasPrefix(importPath, forbidden.importPath+"/") {
					continue
				}
				t.Fatalf(
					"%s must not import %s %s; use %s or %s",
					packagePath,
					forbidden.label,
					importPath,
					factoryDefinitionsOwnerPrefix+"/wire",
					forbiddenOwnerValidationInternalsImport,
				)
			}
		}
	}
}

// TestOwnerProductionPackages_ImportValidationInternalsOnlyThroughAllowedSeams
// fails closed when owner production packages import validation internals
// outside the owner wire bridge, validation subservice tree, or transitional
// shim residue owned by DEL-DEF.
func TestOwnerProductionPackages_ImportValidationInternalsOnlyThroughAllowedSeams(t *testing.T) {
	t.Parallel()

	for _, line := range listOwnerProductionPackageImports(t) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		packagePath := fields[0]
		if isAllowedOwnerValidationInternalsImporter(packagePath) {
			continue
		}
		for _, importPath := range fields[1:] {
			if importPath != forbiddenOwnerValidationInternalsImport &&
				!strings.HasPrefix(importPath, forbiddenOwnerValidationInternalsImport+"/") {
				continue
			}
			t.Fatalf(
				"%s must not import validation internals %s; use %s or %s",
				packagePath,
				importPath,
				factoryDefinitionsOwnerPrefix,
				factoryDefinitionsOwnerPrefix+"/wire",
			)
		}
	}
}

func listOwnerProductionPackageImports(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}} {{join .Imports \" \"}}",
		factoryDefinitionsOwnerPrefix+"/...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory definitions packages: %v\n%s", err, output)
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n")
}

func isAllowedOwnerTransitionalValidationImporter(packagePath string) bool {
	switch {
	case packagePath == transitionalValidationImport,
		strings.HasPrefix(packagePath, transitionalValidationImport+"/"):
		return true
	case packagePath == transitionalNamevalueImport,
		strings.HasPrefix(packagePath, transitionalNamevalueImport+"/"):
		return true
	case packagePath == transitionalTaxonomyImport,
		strings.HasPrefix(packagePath, transitionalTaxonomyImport+"/"):
		return true
	default:
		return false
	}
}

func isAllowedOwnerValidationInternalsImporter(packagePath string) bool {
	switch {
	case packagePath == forbiddenOwnerValidationInternalsImport,
		strings.HasPrefix(packagePath, forbiddenOwnerValidationInternalsImport+"/"):
		return true
	case packagePath == factoryDefinitionsOwnerPrefix+"/wire",
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/wire/"):
		return true
	case packagePath == transitionalValidationImport,
		strings.HasPrefix(packagePath, transitionalValidationImport+"/"):
		return true
	case packagePath == transitionalNamevalueImport,
		strings.HasPrefix(packagePath, transitionalNamevalueImport+"/"):
		return true
	case packagePath == transitionalTaxonomyImport,
		strings.HasPrefix(packagePath, transitionalTaxonomyImport+"/"):
		return true
	case packagePath == factoryDefinitionsOwnerPrefix+"/internal/testcomposition",
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/internal/testcomposition/"):
		return true
	case packagePath == factoryDefinitionsOwnerPrefix+"/definition",
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/definition/"):
		return true
	case packagePath == factoryDefinitionsOwnerPrefix+"/internal",
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/internal/"):
		return true
	case packagePath == factoryDefinitionsOwnerPrefix+"/workers",
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/workers/"):
		return true
	default:
		return false
	}
}
