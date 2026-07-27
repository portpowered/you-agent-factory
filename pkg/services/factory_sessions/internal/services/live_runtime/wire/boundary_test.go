package wire_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_LiveRuntimeWireDoesNotImportProcessCompositionOrPeerServiceWires(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/root",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire",
		"github.com/portpowered/infinite-you/pkg/services/work/wire",
		"github.com/portpowered/infinite-you/pkg/services/workers/wire",
		"github.com/portpowered/infinite-you/pkg/services/models/wire",
		"github.com/portpowered/infinite-you/pkg/services/recordings/wire",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("live_runtime wire must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}

func TestPackageBoundary_LiveRuntimeServiceDoesNotImportProcessCompositionOrPeerServiceWires(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/internal/service",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/root",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire",
		"github.com/portpowered/infinite-you/pkg/services/work/wire",
		"github.com/portpowered/infinite-you/pkg/services/workers/wire",
		"github.com/portpowered/infinite-you/pkg/services/models/wire",
		"github.com/portpowered/infinite-you/pkg/services/recordings/wire",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("live_runtime service must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}
