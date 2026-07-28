package factory_transformation

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryTransformation_CreateNamedFactoryPreservesPortableLayoutThroughActivationAndReadback(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)

	created := createNamedFactoryFromBody(
		t,
		server.URL(),
		functionalNamedFactoryBodyWithPortableLayout("beta", "beta-task"),
	)
	assertNamedFactoryPortableLayoutResponse(t, created.Layout, "workstation:plan-task", "beta-task")

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertNamedFactoryPortableLayoutResponse(t, current.Layout, "workstation:plan-task", "beta-task")

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, "beta", interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(beta/factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(beta/factory.json): %v", err)
	}
	assertPortableLayoutPayload(t, persisted["layout"])

	submitWorkAndExpectStatus(t, server.URL(), "beta-task", "layout-named-factory", http.StatusCreated)
}

func TestFactoryTransformation_UpsertNamedFactoryReplacePreservesPortableLayout(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	created := createNamedFactoryFromBody(
		t,
		server.URL(),
		functionalNamedFactoryBodyWithPortableLayout("beta", "beta-task"),
	)
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	freshVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced := upsertNamedFactoryFromBody(
		t,
		server.URL(),
		currentFactorySaveDocumentWithPortableLayout(t, "beta", "beta-task", versionDocument(freshVersion)),
	)
	assertNamedFactoryPortableLayoutResponse(t, replaced.Layout, "workstation:plan-task", "beta-task")

	current := getCurrentFactory(t, server.URL())
	assertNamedFactoryPortableLayoutResponse(t, current.Layout, "workstation:plan-task", "beta-task")
}

func functionalNamedFactoryBodyWithPortableLayout(name, workType string) string {
	return currentFactorySaveDocumentWithPortableLayout(nil, name, workType, nil)
}

func currentFactorySaveDocumentWithPortableLayout(t *testing.T, name, workType string, version any) string {
	if t != nil {
		t.Helper()
	}
	document := map[string]any{
		"name": name,
		"id":   name,
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id":       "workstation:plan-task",
				"position": map[string]any{"x": 144, "y": 288},
				"size":     map[string]any{"width": 320, "height": 180},
				"locked":   true,
			}},
			"edges": []map[string]any{{
				"id":        "workstation-output:workstation:plan-task->work-state:" + workType + ":done",
				"waypoints": []map[string]any{{"x": 200, "y": 300}},
			}},
			"groups": []map[string]any{{
				"id":      "group-1",
				"label":   "Planning",
				"nodeIds": []string{"workstation:plan-task"},
				"bounds":  map[string]any{"x": 100, "y": 220, "width": 420, "height": 240},
			}},
			"viewport":    map[string]any{"x": 40, "y": 60, "zoom": 0.85},
			"preferences": map[string]any{"direction": "RIGHT"},
		},
		"workTypes": []map[string]any{{
			"name": workType,
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
			"inputs":   []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":  []map[string]string{{"workType": workType, "state": "done"}},
		}},
	}
	if version != nil {
		document["version"] = version
	}
	body, err := json.Marshal(document)
	if err != nil {
		if t != nil {
			t.Fatalf("marshal portable layout factory document: %v", err)
		}
		panic(err)
	}
	return string(body)
}

func assertNamedFactoryPortableLayoutResponse(t *testing.T, layout *factoryapi.FactoryLayout, wantNodeID, wantWorkType string) {
	t.Helper()

	if layout == nil {
		t.Fatal("expected named-factory response layout")
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 || (*layout.Nodes)[0].Id != wantNodeID {
		t.Fatalf("layout nodes = %#v, want %s", layout.Nodes, wantNodeID)
	}
	wantEdgeID := "workstation-output:workstation:plan-task->work-state:" + wantWorkType + ":done"
	if layout.Edges == nil || len(*layout.Edges) != 1 || (*layout.Edges)[0].Id != wantEdgeID {
		t.Fatalf("layout edges = %#v, want %s", layout.Edges, wantEdgeID)
	}
	if layout.Groups == nil || len(*layout.Groups) != 1 || (*layout.Groups)[0].Id != "group-1" {
		t.Fatalf("layout groups = %#v, want group-1", layout.Groups)
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
