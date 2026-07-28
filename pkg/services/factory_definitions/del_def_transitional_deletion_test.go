package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

var deletedTransitionalImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedfactories",
}

var allowedDeletedTransitionalImporterPrefixes = []string{
	"github.com/portpowered/infinite-you/pkg/wire",
}

// TestDelDefTransitionalDeletion_NoImportsOfDeletedPackages proves DEL-DEF
// story 002 cleared production and test imports of deleted transitional
// packages, except the documented root pkg/wire residual for deleted
// transitional import paths retargeted in later DEF fold work.
func TestDelDefTransitionalDeletion_NoImportsOfDeletedPackages(t *testing.T) {
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
		if isAllowedDeletedTransitionalImporter(packagePath) {
			continue
		}
		for _, importPath := range fields[1:] {
			for _, deleted := range deletedTransitionalImports {
				if importPath != deleted && !strings.HasPrefix(importPath, deleted+"/") {
					continue
				}
				t.Fatalf(
					"%s must not import deleted transitional package %s; use internal/services/* owner paths or factory_definitions/wire",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func isAllowedDeletedTransitionalImporter(packagePath string) bool {
	for _, allowed := range allowedDeletedTransitionalImporterPrefixes {
		if packagePath == allowed || strings.HasPrefix(packagePath, allowed+"/") {
			return true
		}
	}
	return false
}
