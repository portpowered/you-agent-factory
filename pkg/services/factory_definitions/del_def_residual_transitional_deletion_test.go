package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

var deletedTransitionalResidualImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loadedsource",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/namevalue",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/replayconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/decisionenvelope",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationinterpolation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationoutput",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationworktype",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/workpropagation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/workstationexecution",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/ttsobservability",
}

// TestDelDefResidualTransitionalDeletion_NoImportsOfDeletedPackages proves
// DEL-DEF-RESIDUAL story 002 cleared production and test imports of deleted
// residual transitional packages.
func TestDelDefResidualTransitionalDeletion_NoImportsOfDeletedPackages(t *testing.T) {
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
		for _, importPath := range fields[1:] {
			for _, deleted := range deletedTransitionalResidualImports {
				if importPath != deleted && !strings.HasPrefix(importPath, deleted+"/") {
					continue
				}
				t.Fatalf(
					"%s must not import deleted residual transitional package %s; use internal/services/* owner paths or factory_definitions/wire",
					packagePath,
					importPath,
				)
			}
		}
	}
}
