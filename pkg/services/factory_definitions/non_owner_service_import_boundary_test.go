package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	moduleImportPrefix              = "github.com/portpowered/infinite-you/"
	factoryDefinitionsOwnerPrefix   = moduleImportPrefix + "pkg/services/factory_definitions"
	rootPkgWireResidualImportPrefix = moduleImportPrefix + "pkg/wire"
)

// TestNonOwnerProductionPackages_DoNotImportTransitionalServiceShim seals
// pss-cln-def-fold-toplevel story 005: only the Factory Definitions owner and
// the documented Automations-leased root pkg/wire residual may import the
// transitional factory_definitions/service compile shim.
func TestNonOwnerProductionPackages_DoNotImportTransitionalServiceShim(t *testing.T) {
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
		if isAllowedTransitionalServiceImporter(packagePath) {
			continue
		}
		for _, importPath := range fields[1:] {
			if importPath != transitionalServiceImport &&
				!strings.HasPrefix(importPath, transitionalServiceImport+"/") {
				continue
			}
			t.Fatalf(
				"%s must not import transitional service shim %s; use %s or the owner wire bridge %s (root pkg/wire residual only)",
				packagePath,
				importPath,
				factoryDefinitionsOwnerPrefix,
				rootPkgWireResidualImportPrefix,
			)
		}
	}
}

func isAllowedTransitionalServiceImporter(packagePath string) bool {
	if packagePath == factoryDefinitionsOwnerPrefix ||
		strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/") {
		return true
	}
	return packagePath == rootPkgWireResidualImportPrefix ||
		strings.HasPrefix(packagePath, rootPkgWireResidualImportPrefix+"/")
}
