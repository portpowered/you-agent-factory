package layouttests

import (
	"math"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func validLayoutFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/guide.md",
			}},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "executor"}},
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
	if len(cfg.Layout.Groups) != 0 {
		t.Fatalf("groups after geometry rejection = %#v, want []", cfg.Layout.Groups)
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

func TestValidate_UnknownBundledDocLayoutNodeBlocksTopologyValidation(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "doc:factory/docs/missing.md",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})

	result := factoryvalidation.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected unknown bundled doc layout node to block save")
	}
	validationassert.HasDomainTargetCode(t, result.BlockingTargets(), factoryvalidation.CodeLayoutUnknownNodeReference)
}

func TestValidate_EmptyStateRequiresCanonicalTopologyNode(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:missing",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
		EmptyState: &interfaces.FactoryLayoutEmptyStateConfig{
			Text: "No activity yet.",
		},
	})

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.BlockingTargets(), factoryvalidation.CodeLayoutEmptyStateUnknownNodeReference)
	for _, target := range result.BlockingTargets() {
		if target.Code == factoryvalidation.CodeLayoutEmptyStateUnknownNodeReference && target.Path != "factory.layout.nodes[1].emptyState" {
			t.Fatalf("empty-state target path = %q", target.Path)
		}
	}
}

func TestValidate_LegacyBundledScriptDocLayoutNodeMatchesPendingTopology(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.ResourceManifest.BundledFiles = append(cfg.ResourceManifest.BundledFiles, interfaces.BundledFileConfig{
		Type:       interfaces.BundledFileTypeScript,
		TargetPath: "factory/scripts/setup-workspace.py",
	})
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "doc:factory/scripts/setup-workspace.py",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})

	result := factoryvalidation.Validate(cfg)
	if result.HasTargets() {
		t.Fatalf("validation targets = %#v, want none for legacy bundled script layout node", result.Targets)
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

func TestPruneLayout_RejectsEmptyStateForUnknownCanonicalNode(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "workstation:missing",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
		EmptyState: &interfaces.FactoryLayoutEmptyStateConfig{
			Text: "No activity yet.",
		},
	})

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutEmptyStateUnknownNodeReference)
	if len(cfg.Layout.Nodes) != 1 || cfg.Layout.Nodes[0].ID != "workstation:plan-task" {
		t.Fatalf("nodes after empty-state rejection = %#v, want only plan-task", cfg.Layout.Nodes)
	}
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeLayoutEmptyStateUnknownNodeReference && target.Path != "factory.layout.nodes[1].emptyState" {
			t.Fatalf("empty-state target path = %q", target.Path)
		}
	}
}

func TestPruneLayout_PreservesLegacyBundledScriptDocLayoutNode(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.ResourceManifest.BundledFiles = append(cfg.ResourceManifest.BundledFiles, interfaces.BundledFileConfig{
		Type:       interfaces.BundledFileTypeScript,
		TargetPath: "factory/scripts/setup-workspace.py",
	})
	cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
		ID:       "doc:factory/scripts/setup-workspace.py",
		Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2},
	})

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	if result.HasTargets() {
		t.Fatalf("prune targets = %#v, want none for legacy bundled script layout node", result.Targets)
	}
	if len(cfg.Layout.Nodes) != 2 {
		t.Fatalf("nodes after prune = %#v, want workstation and legacy bundled script nodes", cfg.Layout.Nodes)
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

func TestPruneLayout_RejectsInvalidGroupBoundsGeometry(t *testing.T) {
	t.Parallel()

	cfg := validLayoutFactoryConfig()
	cfg.Layout.Groups[0].Bounds.Width = math.NaN()

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result := factoryvalidation.PruneLayout(cfg, topology)

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
	if len(cfg.Layout.Groups) != 0 {
		t.Fatalf("groups after geometry rejection = %#v, want []", cfg.Layout.Groups)
	}
}

func TestPruneLayout_EsotericFailureModes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(cfg *interfaces.FactoryConfig)
		assert func(t *testing.T, cfg *interfaces.FactoryConfig, result factoryvalidation.Result)
	}{
		{
			name: "invalid bundled doc node size rejects only the poisoned document node",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
					ID:       "doc:factory/docs/guide.md",
					Position: interfaces.FactoryLayoutPointConfig{X: 40, Y: 80},
					Size:     &interfaces.FactoryLayoutSizeConfig{Width: math.NaN(), Height: 120},
				})
			},
			assert: func(t *testing.T, cfg *interfaces.FactoryConfig, result factoryvalidation.Result) {
				t.Helper()
				validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
				if len(cfg.Layout.Nodes) != 1 || cfg.Layout.Nodes[0].ID != "workstation:plan-task" {
					t.Fatalf("nodes after bundled doc size rejection = %#v, want only workstation node", cfg.Layout.Nodes)
				}
			},
		},
		{
			name: "valid edge with finite waypoints but non-finite label position is rejected wholesale",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Layout.Edges[0].Waypoints = []interfaces.FactoryLayoutPointConfig{{X: 10, Y: 20}}
				cfg.Layout.Edges[0].LabelPosition = &interfaces.FactoryLayoutPointConfig{X: math.Inf(1), Y: 30}
			},
			assert: func(t *testing.T, cfg *interfaces.FactoryConfig, result factoryvalidation.Result) {
				t.Helper()
				validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
				if len(cfg.Layout.Edges) != 0 {
					t.Fatalf("edges after label position rejection = %#v, want []", cfg.Layout.Edges)
				}
			},
		},
		{
			name: "group member pruning preserves duplicate valid members while removing blanks and unknowns",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Layout.Groups[0].NodeIDs = []string{
					"workstation:plan-task",
					"",
					"workstation:missing",
					"workstation:plan-task",
				}
			},
			assert: func(t *testing.T, cfg *interfaces.FactoryConfig, result factoryvalidation.Result) {
				t.Helper()
				validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutUnknownGroupMemberReference)
				want := []string{"workstation:plan-task", "workstation:plan-task"}
				if len(cfg.Layout.Groups) != 1 || strings.Join(cfg.Layout.Groups[0].NodeIDs, ",") != strings.Join(want, ",") {
					t.Fatalf("group members after pruning = %#v, want %#v", cfg.Layout.Groups, want)
				}
			},
		},
		{
			name: "viewport rejection leaves otherwise valid layout entities intact",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Layout.Viewport = &interfaces.FactoryLayoutViewportConfig{X: 0, Y: 0, Zoom: math.NaN()}
				cfg.Layout.Nodes = append(cfg.Layout.Nodes, interfaces.FactoryLayoutNodeConfig{
					ID:       "doc:factory/docs/guide.md",
					Position: interfaces.FactoryLayoutPointConfig{X: 12, Y: 24},
				})
			},
			assert: func(t *testing.T, cfg *interfaces.FactoryConfig, result factoryvalidation.Result) {
				t.Helper()
				validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeLayoutInvalidGeometry)
				if cfg.Layout.Viewport != nil {
					t.Fatalf("viewport after rejection = %#v, want nil", cfg.Layout.Viewport)
				}
				if len(cfg.Layout.Nodes) != 2 {
					t.Fatalf("nodes after viewport-only rejection = %#v, want both valid nodes preserved", cfg.Layout.Nodes)
				}
				if len(cfg.Layout.Edges) != 1 || len(cfg.Layout.Groups) != 1 {
					t.Fatalf("layout after viewport-only rejection = %#v, want edges/groups preserved", cfg.Layout)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validLayoutFactoryConfig()
			tc.mutate(cfg)

			topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
			result := factoryvalidation.PruneLayout(cfg, topology)

			tc.assert(t, cfg, result)
		})
	}
}
