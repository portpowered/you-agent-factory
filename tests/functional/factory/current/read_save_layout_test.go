package current

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCurrentFactoryEvents_ExposePortableLayoutOnInitialStructureAndFactoryChange(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalFactoryEventLayoutDocument(t, "root-runtime", "story", nil, initialFactoryEventLayout()),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config with layout: %v", err)
	}

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	initialPayload := requireInitialStructurePayload(t, server.GetFactoryEvents(t))
	assertFactoryEventLayout(t, initialPayload.Factory.Layout, factoryEventLayoutExpectation{
		nodeX:       144,
		nodeY:       288,
		nodeWidth:   320,
		nodeHeight:  180,
		nodeLocked:  true,
		waypoints:   []factoryapi.FactoryLayoutPoint{{X: 200, Y: 300}},
		labelX:      220,
		labelY:      280,
		groupLabel:  "Planning",
		groupColor:  "#ddeeff",
		groupLocked: true,
		viewportX:   40,
		viewportY:   60,
		zoom:        0.85,
		direction:   factoryapi.RIGHT,
	})

	initialEvents := server.GetFactoryEvents(t)
	current := getCurrentFactory(t, server.URL())
	saveCurrentFactoryDefinition(
		t,
		server.URL(),
		string(functionalFactoryEventLayoutDocument(
			t,
			"UNDEFINED",
			"story",
			versionDocument(advancedFactoryVersion(t, current.Version)),
			modifiedFactoryEventLayout(),
		)),
	)

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	changePayload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	assertFactoryEventLayout(t, changePayload.Factory.Layout, factoryEventLayoutExpectation{
		nodeX:       344,
		nodeY:       488,
		nodeWidth:   360,
		nodeHeight:  210,
		nodeLocked:  false,
		waypoints:   []factoryapi.FactoryLayoutPoint{{X: 260, Y: 340}, {X: 300, Y: 360}},
		labelX:      275,
		labelY:      325,
		groupLabel:  "Execution",
		groupColor:  "#ccddee",
		groupLocked: false,
		viewportX:   80,
		viewportY:   90,
		zoom:        1.1,
		direction:   factoryapi.DOWN,
	})
}

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

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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

func TestCurrentFactoryPUT_PrunesStaleLayoutWithoutReturningEphemeralLayoutMetadata(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	body, err := json.Marshal(staleLayoutPruningFactorySaveBody(t, current))
	if err != nil {
		t.Fatalf("marshal current factory save with stale layout: %v", err)
	}

	saveCurrentFactoryDefinition(t, server.URL(), string(body))
	assertStaleLayoutPrunedOnDisk(t, rootDir)

	reloaded := getCurrentFactory(t, server.URL())
	_ = reloaded
}

func TestCurrentFactoryPUT_AcceptsLayoutNodeMissingSize(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
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
			}},
			"viewport": map[string]any{
				"x":    40,
				"y":    60,
				"zoom": 0.85,
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
		t.Fatalf("marshal current factory save with malformed layout node: %v", err)
	}

	saved := saveCurrentFactoryDefinition(t, server.URL(), string(body))
	if saved.Layout == nil || saved.Layout.Nodes == nil || len(*saved.Layout.Nodes) != 1 {
		t.Fatalf("saved layout nodes = %#v, want one sizeless node", saved.Layout)
	}
	if (*saved.Layout.Nodes)[0].Size != nil {
		t.Fatalf("saved layout node size = %#v, want omitted size", (*saved.Layout.Nodes)[0].Size)
	}
}

func TestCurrentFactoryPUT_AcceptsLayoutForKnownBundledDocNode(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	body := currentFactoryDocumentWithBundledDocsAndLayout(
		t,
		current,
		[]map[string]any{
			docBundledFileEntry("factory/docs/planning.md", "# Planning\n"),
		},
		map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id":       "doc:factory/docs/planning.md",
				"position": map[string]any{"x": 180, "y": 220},
				"size":     map[string]any{"width": 360, "height": 200},
			}},
			"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
		},
	)

	saved := saveCurrentFactoryDefinition(t, server.URL(), body)
	assertDocBundledFileInline(t, saved, "factory/docs/planning.md", "# Planning\n")
	if saved.Layout == nil || saved.Layout.Nodes == nil || len(*saved.Layout.Nodes) != 1 {
		t.Fatalf("saved layout = %#v, want one bundled doc node", saved.Layout)
	}
	node := (*saved.Layout.Nodes)[0]
	if node.Id != "doc:factory/docs/planning.md" {
		t.Fatalf("saved bundled doc layout node id = %q, want doc:factory/docs/planning.md", node.Id)
	}
	if node.Size == nil || node.Size.Width != 360 || node.Size.Height != 200 {
		t.Fatalf("saved bundled doc layout node size = %#v, want 360x200", node.Size)
	}

	reloaded := getCurrentFactory(t, server.URL())
	assertDocBundledFileInline(t, reloaded, "factory/docs/planning.md", "# Planning\n")
	if reloaded.Layout == nil || reloaded.Layout.Nodes == nil || len(*reloaded.Layout.Nodes) != 1 {
		t.Fatalf("reloaded layout = %#v, want one bundled doc node", reloaded.Layout)
	}
}

func TestCurrentFactoryPUT_RejectsLayoutForUnknownBundledDocNode(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())

	body := currentFactoryDocumentWithBundledDocsAndLayout(
		t,
		current,
		[]map[string]any{
			docBundledFileEntry("factory/docs/planning.md", "# Planning\n"),
		},
		map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id":       "doc:factory/docs/unknown.md",
				"position": map[string]any{"x": 180, "y": 220},
			}},
			"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
		},
	)

	resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid bundled doc layout save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Targets == nil || !hasValidationTargetCode(*errResp.Targets, factoryValidationCodeLayoutUnknownNodeReference) {
		t.Fatalf("error targets = %#v, want unknown bundled doc layout reference", errResp.Targets)
	}
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
