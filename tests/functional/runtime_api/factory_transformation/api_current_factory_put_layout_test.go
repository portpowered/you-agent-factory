package factory_transformation

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// backendsizecheck:ignore-function this end-to-end current-factory layout round-trip test keeps save, reload, persistence, and runtime assertions on one contract seam.
func TestCurrentFactoryPUT_PreservesPortableLayoutThroughSaveReloadAndRuntimeExecution(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())

	body, err := json.Marshal(map[string]any{
		"name":    "UNDEFINED",
		"id":      "root-runtime",
		"version": versionDocument(advancedFactoryVersion(t, current.Version)),
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id": "workstation:plan-task",
				"position": map[string]any{
					"x": 144,
					"y": 288,
				},
				"size": map[string]any{
					"width":  320,
					"height": 180,
				},
				"locked": true,
			}},
			"edges": []map[string]any{{
				"id": "workstation-output:workstation:plan-task->work-state:story:done",
				"waypoints": []map[string]any{{
					"x": 200,
					"y": 300,
				}},
			}},
			"groups": []map[string]any{{
				"id":      "group-1",
				"label":   "Planning",
				"nodeIds": []string{"workstation:plan-task"},
				"bounds": map[string]any{
					"x":      100,
					"y":      220,
					"width":  420,
					"height": 240,
				},
			}},
			"viewport": map[string]any{
				"x":    40,
				"y":    60,
				"zoom": 0.85,
			},
			"preferences": map[string]any{
				"direction": "RIGHT",
			},
		},
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
			"body":             "You are the planner.",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"body":     "Plan the work.",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "done"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal current factory save with layout: %v", err)
	}

	saved := saveCurrentFactoryDefinition(t, server.URL(), string(body))
	assertPortableLayoutResponse(t, saved.Layout)

	reloaded := getCurrentFactory(t, server.URL())
	assertPortableLayoutResponse(t, reloaded.Layout)

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	assertPortableLayoutPayload(t, persisted["layout"])

	submitWorkAndExpectStatus(t, server.URL(), "story", "layout-roundtrip", http.StatusCreated)
}

func TestCurrentFactoryPUT_PrunesStaleLayoutAndReturnsLayoutOutcomes(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())

	body, err := json.Marshal(map[string]any{
		"name":    "UNDEFINED",
		"id":      "root-runtime",
		"version": versionDocument(advancedFactoryVersion(t, current.Version)),
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id": "workstation:plan-task",
				"position": map[string]any{
					"x": 144,
					"y": 288,
				},
				"size": map[string]any{
					"width":  320,
					"height": 180,
				},
			}, {
				"id": "workstation:removed-node",
				"position": map[string]any{
					"x": 10,
					"y": 20,
				},
				"size": map[string]any{
					"width":  100,
					"height": 80,
				},
			}},
			"edges": []map[string]any{{
				"id": "workstation-output:workstation:plan-task->work-state:story:done",
			}, {
				"id": "workstation-output:workstation:removed-node->work-state:story:done",
			}},
			"groups": []map[string]any{{
				"id":      "group-1",
				"nodeIds": []string{"workstation:plan-task", "workstation:removed-node"},
				"bounds": map[string]any{
					"x": 0, "y": 0, "width": 100, "height": 80,
				},
			}, {
				"id":      "group-empty",
				"nodeIds": []string{"workstation:removed-node"},
				"bounds": map[string]any{
					"x": 0, "y": 0, "width": 50, "height": 50,
				},
			}},
			"viewport": map[string]any{
				"x": 0, "y": 0, "zoom": 1,
			},
		},
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
			"body":             "You are the planner.",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"body":     "Plan the work.",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "done"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal current factory save with stale layout: %v", err)
	}

	saved := saveCurrentFactoryDefinition(t, server.URL(), string(body))

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	layout, ok := persisted["layout"].(map[string]any)
	if !ok {
		t.Fatalf("persisted layout = %#v, want object", persisted["layout"])
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("persisted layout nodes = %#v, want one pruned node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:plan-task" {
		t.Fatalf("persisted layout node = %#v, want workstation:plan-task", nodes[0])
	}

	if saved.LayoutOutcomes == nil || len(*saved.LayoutOutcomes) == 0 {
		t.Fatal("expected layoutOutcomes on save response")
	}
	if !hasValidationTargetCode(*saved.LayoutOutcomes, factoryvalidation.CodeLayoutUnknownNodeReference) ||
		!hasValidationTargetCode(*saved.LayoutOutcomes, factoryvalidation.CodeLayoutUnknownEdgeReference) ||
		!hasValidationTargetCode(*saved.LayoutOutcomes, factoryvalidation.CodeLayoutUnknownGroupMemberReference) {
		t.Fatalf("layoutOutcomes = %#v, want stale layout pruning targets", *saved.LayoutOutcomes)
	}

	groups, ok := layout["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("persisted layout groups = %#v, want empty group preserved", layout["groups"])
	}

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.LayoutOutcomes != nil {
		t.Fatalf("reload layoutOutcomes = %#v, want omitted on GET", reloaded.LayoutOutcomes)
	}
}

func assertPortableLayoutResponse(t *testing.T, layout *factoryapi.FactoryLayout) {
	t.Helper()

	if layout == nil {
		t.Fatal("expected current-factory response layout")
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 || (*layout.Nodes)[0].Id != "workstation:plan-task" {
		t.Fatalf("layout nodes = %#v, want workstation:plan-task", layout.Nodes)
	}
	if (*layout.Nodes)[0].Position.X != 144 || (*layout.Nodes)[0].Position.Y != 288 {
		t.Fatalf("layout node position = %#v, want x=144 y=288", (*layout.Nodes)[0].Position)
	}
	if layout.Edges == nil || len(*layout.Edges) != 1 || (*layout.Edges)[0].Id != "workstation-output:workstation:plan-task->work-state:story:done" {
		t.Fatalf("layout edges = %#v, want plan-task output edge", layout.Edges)
	}
	waypoints := (*layout.Edges)[0].Waypoints
	if waypoints == nil || len(*waypoints) != 1 || (*waypoints)[0].X != 200 {
		t.Fatalf("layout edge waypoints = %#v, want one waypoint at x=200", waypoints)
	}
	if layout.Groups == nil || len(*layout.Groups) != 1 || (*layout.Groups)[0].Id != "group-1" {
		t.Fatalf("layout groups = %#v, want group-1", layout.Groups)
	}
	if len((*layout.Groups)[0].NodeIds) != 1 || (*layout.Groups)[0].NodeIds[0] != "workstation:plan-task" {
		t.Fatalf("layout group nodeIds = %#v, want workstation:plan-task", (*layout.Groups)[0].NodeIds)
	}
	if layout.Viewport == nil || math.Abs(float64(layout.Viewport.Zoom)-0.85) > 1e-6 {
		t.Fatalf("layout viewport = %#v, want zoom 0.85", layout.Viewport)
	}
	if layout.Preferences == nil || layout.Preferences.Direction == nil || *layout.Preferences.Direction != factoryapi.RIGHT {
		t.Fatalf("layout preferences = %#v, want RIGHT", layout.Preferences)
	}
}

func assertPortableLayoutPayload(t *testing.T, value any) {
	t.Helper()

	layout, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("persisted layout = %#v, want object", value)
	}
	if got := layout["schemaVersion"]; got != float64(1) {
		t.Fatalf("persisted layout schemaVersion = %#v, want 1", got)
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("persisted layout nodes = %#v, want one node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:plan-task" {
		t.Fatalf("persisted layout node = %#v, want workstation:plan-task", nodes[0])
	}
	edges, ok := layout["edges"].([]any)
	if !ok || len(edges) != 1 {
		t.Fatalf("persisted layout edges = %#v, want one edge", layout["edges"])
	}
	groups, ok := layout["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("persisted layout groups = %#v, want one group", layout["groups"])
	}
	viewport, ok := layout["viewport"].(map[string]any)
	if !ok || viewport["zoom"] != 0.85 {
		t.Fatalf("persisted layout viewport = %#v, want zoom 0.85", layout["viewport"])
	}
	preferences, ok := layout["preferences"].(map[string]any)
	if !ok || preferences["direction"] != "RIGHT" {
		t.Fatalf("persisted layout preferences = %#v, want RIGHT", layout["preferences"])
	}
}
