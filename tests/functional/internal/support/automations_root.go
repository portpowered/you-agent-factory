package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// AutomationsRootFromProcessEdges returns the published Automations Root composed
// through the same InjectBundle wiring used by BuildProcess.
func AutomationsRootFromProcessEdges(
	t testing.TB,
	edges serviceedges.Edges,
	factoryDir string,
) automations.Root {
	t.Helper()

	root, err := wire.AutomationsRootFromEdges(edges, "functional-automations", factoryDir)
	if err != nil {
		t.Fatalf("AutomationsRootFromEdges() error = %v", err)
	}
	if root.Operations == nil {
		t.Fatal("AutomationsRootFromEdges() returned root without operations")
	}
	return root
}
