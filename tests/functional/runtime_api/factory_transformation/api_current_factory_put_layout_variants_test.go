package factory_transformation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type portableLayoutVariantExpectation struct {
	nodes               []map[string]any
	edges               []map[string]any
	assertSavedLayout   func(t *testing.T, layout *factoryapi.FactoryLayout)
	assertPersistedBody func(t *testing.T, layout map[string]any)
}

func TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithSize(t *testing.T) {
	runCurrentFactoryPUTPortableLayoutVariant(t, portableLayoutVariantExpectation{
		nodes: []map[string]any{
			{
				"id":       "workstation:plan-task",
				"position": map[string]any{"x": 144, "y": 288},
				"size":     map[string]any{"width": 320, "height": 180},
			},
			{
				"id":       "workstation:review-task",
				"position": map[string]any{"x": 544, "y": 288},
				"size":     map[string]any{"width": 300, "height": 160},
			},
		},
		edges: []map[string]any{
			{"id": "workstation-output:workstation:plan-task->work-state:story:draft"},
			{"id": "workstation-output:workstation:review-task->work-state:story:done"},
		},
		assertSavedLayout: func(t *testing.T, layout *factoryapi.FactoryLayout) {
			t.Helper()
			if layout == nil || layout.Nodes == nil || len(*layout.Nodes) != 2 {
				t.Fatalf("layout nodes = %#v, want 2", layout)
			}
			if (*layout.Nodes)[0].Size == nil || (*layout.Nodes)[0].Size.Width != 320 {
				t.Fatalf("first layout node size = %#v, want width 320", (*layout.Nodes)[0].Size)
			}
			if (*layout.Nodes)[1].Size == nil || (*layout.Nodes)[1].Size.Height != 160 {
				t.Fatalf("second layout node size = %#v, want height 160", (*layout.Nodes)[1].Size)
			}
		},
		assertPersistedBody: func(t *testing.T, layout map[string]any) {
			t.Helper()
			nodes := layout["nodes"].([]any)
			second := nodes[1].(map[string]any)
			size := second["size"].(map[string]any)
			if size["width"] != float64(300) || size["height"] != float64(160) {
				t.Fatalf("persisted second node size = %#v, want width 300 height 160", size)
			}
		},
	})
}

func TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithOneWaypoint(t *testing.T) {
	runCurrentFactoryPUTPortableLayoutVariant(t, portableLayoutVariantExpectation{
		nodes: layoutVariantSizedNodes(),
		edges: []map[string]any{{
			"id":        "workstation-output:workstation:plan-task->work-state:story:draft",
			"waypoints": []map[string]any{{"x": 420, "y": 310}},
		}},
		assertSavedLayout: func(t *testing.T, layout *factoryapi.FactoryLayout) {
			t.Helper()
			waypoints := (*layout.Edges)[0].Waypoints
			if waypoints == nil || len(*waypoints) != 1 || (*waypoints)[0].X != 420 || (*waypoints)[0].Y != 310 {
				t.Fatalf("layout edge waypoints = %#v, want one waypoint at 420,310", waypoints)
			}
		},
		assertPersistedBody: func(t *testing.T, layout map[string]any) {
			t.Helper()
			assertPersistedWaypointCount(t, layout, 1)
		},
	})
}

func TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithMultipleWaypoints(t *testing.T) {
	runCurrentFactoryPUTPortableLayoutVariant(t, portableLayoutVariantExpectation{
		nodes: layoutVariantSizedNodes(),
		edges: []map[string]any{{
			"id": "workstation-output:workstation:review-task->work-state:story:done",
			"waypoints": []map[string]any{
				{"x": 700, "y": 260},
				{"x": 760, "y": 220},
			},
		}},
		assertSavedLayout: func(t *testing.T, layout *factoryapi.FactoryLayout) {
			t.Helper()
			waypoints := (*layout.Edges)[0].Waypoints
			if waypoints == nil || len(*waypoints) != 2 {
				t.Fatalf("layout edge waypoints = %#v, want 2", waypoints)
			}
			if (*waypoints)[1].X != 760 || (*waypoints)[1].Y != 220 {
				t.Fatalf("second layout waypoint = %#v, want 760,220", (*waypoints)[1])
			}
		},
		assertPersistedBody: func(t *testing.T, layout map[string]any) {
			t.Helper()
			assertPersistedWaypointCount(t, layout, 2)
		},
	})
}

func TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithoutSize(t *testing.T) {
	runCurrentFactoryPUTPortableLayoutVariant(t, portableLayoutVariantExpectation{
		nodes: []map[string]any{
			{"id": "workstation:plan-task", "position": map[string]any{"x": 144, "y": 288}},
			{"id": "workstation:review-task", "position": map[string]any{"x": 544, "y": 288}},
		},
		edges: []map[string]any{{"id": "workstation-output:workstation:plan-task->work-state:story:draft"}},
		assertSavedLayout: func(t *testing.T, layout *factoryapi.FactoryLayout) {
			t.Helper()
			if layout == nil || layout.Nodes == nil || len(*layout.Nodes) != 2 {
				t.Fatalf("layout nodes = %#v, want 2", layout)
			}
			if (*layout.Nodes)[0].Size != nil || (*layout.Nodes)[1].Size != nil {
				t.Fatalf("layout node sizes = %#v, want omitted sizes", *layout.Nodes)
			}
		},
		assertPersistedBody: func(t *testing.T, layout map[string]any) {
			t.Helper()
			nodes := layout["nodes"].([]any)
			for _, nodeValue := range nodes {
				node := nodeValue.(map[string]any)
				if _, ok := node["size"]; ok {
					t.Fatalf("persisted sizeless node = %#v, want size omitted", node)
				}
			}
		},
	})
}

func runCurrentFactoryPUTPortableLayoutVariant(t *testing.T, variant portableLayoutVariantExpectation) {
	t.Helper()

	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startDocumentTransformationServer(t, rootDir, "")
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	body, err := json.Marshal(layoutVariantFactorySaveBody(t, current, variant.nodes, variant.edges))
	if err != nil {
		t.Fatalf("marshal current factory save with layout variant: %v", err)
	}

	saved := saveCurrentFactoryForSession(t, server.URL(), server.SessionID(), string(body))
	variant.assertSavedLayout(t, saved.Layout)

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	variant.assertSavedLayout(t, reloaded.Layout)

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	layout := persisted["layout"].(map[string]any)
	variant.assertPersistedBody(t, layout)
}

func layoutVariantSizedNodes() []map[string]any {
	return []map[string]any{
		{
			"id":       "workstation:plan-task",
			"position": map[string]any{"x": 144, "y": 288},
			"size":     map[string]any{"width": 320, "height": 180},
		},
		{
			"id":       "workstation:review-task",
			"position": map[string]any{"x": 544, "y": 288},
			"size":     map[string]any{"width": 300, "height": 160},
		},
	}
}

func assertPersistedWaypointCount(t *testing.T, layout map[string]any, want int) {
	t.Helper()

	edges := layout["edges"].([]any)
	waypoints := edges[0].(map[string]any)["waypoints"].([]any)
	if len(waypoints) != want {
		t.Fatalf("persisted waypoints = %#v, want %d", waypoints, want)
	}
}

func layoutVariantFactorySaveBody(
	t *testing.T,
	current factoryapi.Factory,
	nodes []map[string]any,
	edges []map[string]any,
) map[string]any {
	t.Helper()

	return map[string]any{
		"name":    "UNDEFINED",
		"id":      "root-runtime",
		"version": versionDocument(advancedFactoryVersion(t, current.Version)),
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes":         nodes,
			"edges":         edges,
			"viewport":      map[string]any{"x": 40, "y": 60, "zoom": 0.85},
			"preferences":   map[string]any{"direction": "RIGHT"},
		},
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "draft", "type": "PROCESSING"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": layoutVariantWorkers(),
		"workstations": []map[string]any{
			{
				"name":     "plan-task",
				"behavior": "STANDARD",
				"type":     "MODEL_WORKSTATION",
				"worker":   "planner",
				"body":     "Plan the work.",
				"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "story", "state": "draft"}},
			},
			{
				"name":     "review-task",
				"behavior": "STANDARD",
				"type":     "MODEL_WORKSTATION",
				"worker":   "reviewer",
				"body":     "Review the work.",
				"inputs":   []map[string]string{{"workType": "story", "state": "draft"}},
				"outputs":  []map[string]string{{"workType": "story", "state": "done"}},
			},
		},
	}
}

func layoutVariantWorkers() []map[string]any {
	return []map[string]any{
		{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
			"body":             "You are the planner.",
		},
		{
			"name":             "reviewer",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
			"body":             "You are the reviewer.",
		},
	}
}
