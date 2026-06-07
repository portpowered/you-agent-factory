package validation_test

import (
	"math"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestPruneLayout_RemovesStaleNodeEdgeAndGroupMemberReferences(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:missing",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})
	cfg.Layout.Edges = append(cfg.Layout.Edges, interfaces.FactoryLayoutEdgeConfig{
		ID: "workstation-output:workstation:missing->work-state:story:done",
	})
	cfg.Layout.Groups = append(cfg.Layout.Groups, interfaces.FactoryLayoutGroupConfig{
		ID:      "group-empty",
		NodeIDs: []string{"workstation:missing"},
		Bounds:  interfaces.FactoryLayoutBoundsConfig{X: 0, Y: 0, Width: 10, Height: 10},
	})
	cfg.Layout.Groups[0].NodeIDs = append(cfg.Layout.Groups[0].NodeIDs, "workstation:missing")

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownNodeReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownEdgeReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownGroupMemberReference)
	if len(cfg.Layout.Nodes) != 1 || cfg.Layout.Nodes[0].ID != "workstation:plan-task" {
		t.Fatalf("pruned nodes = %#v, want only plan-task", cfg.Layout.Nodes)
	}
	if len(cfg.Layout.Edges) != 1 {
		t.Fatalf("pruned edges = %#v, want only valid edge", cfg.Layout.Edges)
	}
	if len(cfg.Layout.Groups) != 2 {
		t.Fatalf("groups = %#v, want empty group preserved", cfg.Layout.Groups)
	}
	if len(cfg.Layout.Groups[1].NodeIDs) != 0 {
		t.Fatalf("empty group nodeIds = %#v, want []", cfg.Layout.Groups[1].NodeIDs)
	}
	if len(cfg.Layout.Groups[0].NodeIDs) != 1 || cfg.Layout.Groups[0].NodeIDs[0] != "workstation:plan-task" {
		t.Fatalf("group nodeIds = %#v, want only plan-task", cfg.Layout.Groups[0].NodeIDs)
	}
}

func TestPruneLayout_RejectsNonFiniteGeometryConsistently(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes[0].Position.X = math.NaN()

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
	if len(cfg.Layout.Nodes) != 0 {
		t.Fatalf("nodes after geometry rejection = %#v, want []", cfg.Layout.Nodes)
	}
}

func TestPruneLayout_RejectsInvalidViewportGeometry(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Viewport.Zoom = math.Inf(1)

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
	if cfg.Layout.Viewport != nil {
		t.Fatalf("viewport = %#v, want nil after rejection", cfg.Layout.Viewport)
	}
}
