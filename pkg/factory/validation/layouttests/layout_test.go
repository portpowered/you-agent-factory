package layouttests

import (
	"math"
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func validLayoutFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "plan-task",
			Name:           "plan-task",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "done"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		}},
		Layout: &interfaces.FactoryLayoutConfig{
			SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
			Nodes: []interfaces.FactoryLayoutNodeConfig{{
				ID:       "workstation:plan-task",
				Position: interfaces.FactoryLayoutPointConfig{X: 128, Y: 256},
			}},
			Edges: []interfaces.FactoryLayoutEdgeConfig{{
				ID: "workstation-output:workstation:plan-task->work-state:story:done",
			}},
			Groups: []interfaces.FactoryLayoutGroupConfig{{
				ID:      "group-1",
				NodeIDs: []string{"workstation:plan-task"},
				Bounds:  interfaces.FactoryLayoutBoundsConfig{X: 10, Y: 20, Width: 100, Height: 80},
			}},
			Viewport: &interfaces.FactoryLayoutViewportConfig{X: 0, Y: 0, Zoom: 1},
		},
	}
}

func TestValidateLayout_UnknownReferencesAreRecoverableWarnings(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:missing",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})
	cfg.Layout.Edges = append(cfg.Layout.Edges, interfaces.FactoryLayoutEdgeConfig{
		ID: "workstation-output:workstation:missing->work-state:story:done",
	})
	cfg.Layout.Groups[0].NodeIDs = append(cfg.Layout.Groups[0].NodeIDs, "workstation:missing")

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.ValidateLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownNodeReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownEdgeReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownGroupMemberReference)
	for _, target := range result.Targets {
		if target.Severity != factoryvalidation.SeverityWarning {
			t.Fatalf("target severity = %q, want warning for recoverable layout defect", target.Severity)
		}
	}
}

func TestValidateLayout_UnsupportedSchemaVersionIsRecoverableWarning(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.SchemaVersion = 99

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.ValidateLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnsupportedSchemaVersion)
	if result.Targets[0].Severity != factoryvalidation.SeverityWarning {
		t.Fatalf("schemaVersion target severity = %q, want warning", result.Targets[0].Severity)
	}
}

func TestLayoutSaveOutcomes_IncludesUnsupportedSchemaVersionAfterPrune(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.SchemaVersion = 99

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.LayoutSaveOutcomes(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnsupportedSchemaVersion)
}

func TestLayoutSaveOutcomes_PrefersPruneOutcomeForDuplicateCodeAndPath(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Groups[0].Bounds.Width = math.NaN()

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.LayoutSaveOutcomes(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
	for _, target := range result.Targets {
		if target.Code != factoryvalidation.CodeLayoutInvalidGeometry {
			continue
		}
		if !strings.Contains(target.Message, "rejected during save") {
			t.Fatalf("geometry target message = %q, want prune rejection wording", target.Message)
		}
	}
}

func TestValidateLayout_InvalidGeometryIdentifiesAffectedLayoutObject(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes[0].Position.X = math.NaN()

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.ValidateLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeFactory,
		ID:       "nodes[0]",
		Location: factoryvalidation.SubjectLocationReference,
	})
}

func TestValidate_LayoutWarningsDoNotBlockTopologyValidation(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:stale",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})

	result := factoryvalidation.Validate(cfg)
	if !result.HasTargets() {
		t.Fatal("expected layout warnings in validation result")
	}
	if result.HasBlockingTargets() {
		t.Fatalf("blocking targets = %#v, want only recoverable layout warnings", result.BlockingTargets())
	}
}

func TestValidate_InvalidTopologyStillReportsBlockingTargetsSeparatelyFromLayout(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Workstations[0].WorkerTypeName = "missing-worker"
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:stale",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})

	result := factoryvalidation.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking topology validation target")
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDanglingWorkerReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownNodeReference)
}

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
