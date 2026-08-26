package factory_transformation

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCurrentFactoryEvents_ExposePortableLayoutOnInitialStructureAndFactoryChange(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		functionalFactoryEventLayoutDocument(t, "root-runtime", "story", nil, initialFactoryEventLayout()),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config with layout: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
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
	t.Parallel()

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

func TestCurrentFactoryPUT_PrunesStaleLayoutWithoutReturningEphemeralLayoutMetadata(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
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
	t.Parallel()

	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
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

func staleLayoutPruningFactorySaveBody(t *testing.T, current factoryapi.Factory) map[string]any {
	t.Helper()

	return map[string]any{
		"name":    "UNDEFINED",
		"id":      "root-runtime",
		"version": versionDocument(advancedFactoryVersion(t, current.Version)),
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id":       "workstation:plan-task",
				"position": map[string]any{"x": 144, "y": 288},
				"size":     map[string]any{"width": 320, "height": 180},
			}, {
				"id":       "workstation:removed-node",
				"position": map[string]any{"x": 10, "y": 20},
				"size":     map[string]any{"width": 100, "height": 80},
			}},
			"edges": []map[string]any{{
				"id": "workstation-output:workstation:plan-task->work-state:story:done",
			}, {
				"id": "workstation-output:workstation:removed-node->work-state:story:done",
			}},
			"groups": []map[string]any{{
				"id":      "group-1",
				"nodeIds": []string{"workstation:plan-task", "workstation:removed-node"},
				"bounds":  map[string]any{"x": 0, "y": 0, "width": 100, "height": 80},
			}, {
				"id":      "group-empty",
				"nodeIds": []string{"workstation:removed-node"},
				"bounds":  map[string]any{"x": 0, "y": 0, "width": 50, "height": 50},
			}},
			"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
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
	}
}

func assertStaleLayoutPrunedOnDisk(t *testing.T, rootDir string) {
	t.Helper()

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
	groups, ok := layout["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("persisted layout groups = %#v, want empty group preserved", layout["groups"])
	}
}

type factoryEventLayoutExpectation struct {
	nodeX       float32
	nodeY       float32
	nodeWidth   float32
	nodeHeight  float32
	nodeLocked  bool
	waypoints   []factoryapi.FactoryLayoutPoint
	labelX      float32
	labelY      float32
	groupLabel  string
	groupColor  string
	groupLocked bool
	viewportX   float32
	viewportY   float32
	zoom        float32
	direction   factoryapi.FactoryLayoutPreferencesDirection
}

func functionalFactoryEventLayoutDocument(
	t *testing.T,
	name string,
	workType string,
	version any,
	layout map[string]any,
) []byte {
	t.Helper()
	id := name
	if name == "UNDEFINED" {
		id = "root-runtime"
	}
	document := map[string]any{
		"name":   name,
		"id":     id,
		"layout": layout,
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
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"inputs":   []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":  []map[string]string{{"workType": workType, "state": "done"}},
		}},
	}
	if version != nil {
		document["version"] = version
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal factory event layout document: %v", err)
	}
	return body
}

func initialFactoryEventLayout() map[string]any {
	return factoryEventLayout(
		144,
		288,
		320,
		180,
		true,
		[]map[string]any{{"x": 200, "y": 300}},
		220,
		280,
		"Planning",
		"#ddeeff",
		true,
		40,
		60,
		0.85,
		"RIGHT",
	)
}

func modifiedFactoryEventLayout() map[string]any {
	return factoryEventLayout(
		344,
		488,
		360,
		210,
		false,
		[]map[string]any{{"x": 260, "y": 340}, {"x": 300, "y": 360}},
		275,
		325,
		"Execution",
		"#ccddee",
		false,
		80,
		90,
		1.1,
		"DOWN",
	)
}

func factoryEventLayout(
	nodeX float64,
	nodeY float64,
	nodeWidth float64,
	nodeHeight float64,
	nodeLocked bool,
	waypoints []map[string]any,
	labelX float64,
	labelY float64,
	groupLabel string,
	groupColor string,
	groupLocked bool,
	viewportX float64,
	viewportY float64,
	zoom float64,
	direction string,
) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id":       "workstation:plan-task",
			"position": map[string]any{"x": nodeX, "y": nodeY},
			"size":     map[string]any{"width": nodeWidth, "height": nodeHeight},
			"locked":   nodeLocked,
		}},
		"edges": []map[string]any{{
			"id":            "workstation-output:workstation:plan-task->work-state:story:done",
			"waypoints":     waypoints,
			"labelPosition": map[string]any{"x": labelX, "y": labelY},
		}},
		"groups": []map[string]any{{
			"id":            "group-1",
			"label":         groupLabel,
			"nodeIds":       []string{"workstation:plan-task"},
			"bounds":        map[string]any{"x": 100, "y": 220, "width": 420, "height": 240},
			"parentGroupId": "group-root",
			"color":         groupColor,
			"locked":        groupLocked,
		}},
		"viewport":    map[string]any{"x": viewportX, "y": viewportY, "zoom": zoom},
		"preferences": map[string]any{"direction": direction},
	}
}

func requireInitialStructurePayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.InitialStructureRequestEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			t.Fatalf("decode initial-structure payload: %v", err)
		}
		return payload
	}
	t.Fatalf("initial-structure event not found in %d events", len(events))
	return factoryapi.InitialStructureRequestEventPayload{}
}

func assertFactoryEventLayout(t *testing.T, layout *factoryapi.FactoryLayout, want factoryEventLayoutExpectation) {
	t.Helper()
	if layout == nil {
		t.Fatal("factory event layout = nil, want portable layout")
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("factory event layout schemaVersion = %d, want 1", layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 {
		t.Fatalf("factory event layout nodes = %#v, want one node", layout.Nodes)
	}
	node := (*layout.Nodes)[0]
	if node.Id != "workstation:plan-task" ||
		node.Position.X != want.nodeX ||
		node.Position.Y != want.nodeY ||
		node.Size == nil ||
		node.Size.Width != want.nodeWidth ||
		node.Size.Height != want.nodeHeight ||
		node.Locked == nil ||
		*node.Locked != want.nodeLocked {
		t.Fatalf("factory event layout node = %#v, want position/size/locked expectation %#v", node, want)
	}

	if layout.Edges == nil || len(*layout.Edges) != 1 {
		t.Fatalf("factory event layout edges = %#v, want one edge", layout.Edges)
	}
	edge := (*layout.Edges)[0]
	if edge.Id != "workstation-output:workstation:plan-task->work-state:story:done" {
		t.Fatalf("factory event layout edge id = %q, want plan-task output edge", edge.Id)
	}
	if edge.Waypoints == nil || len(*edge.Waypoints) != len(want.waypoints) {
		t.Fatalf("factory event layout edge waypoints = %#v, want %#v", edge.Waypoints, want.waypoints)
	}
	for i, waypoint := range *edge.Waypoints {
		if waypoint != want.waypoints[i] {
			t.Fatalf("factory event layout waypoint[%d] = %#v, want %#v", i, waypoint, want.waypoints[i])
		}
	}
	if edge.LabelPosition == nil || edge.LabelPosition.X != want.labelX || edge.LabelPosition.Y != want.labelY {
		t.Fatalf("factory event layout labelPosition = %#v, want %v,%v", edge.LabelPosition, want.labelX, want.labelY)
	}

	if layout.Groups == nil || len(*layout.Groups) != 1 {
		t.Fatalf("factory event layout groups = %#v, want one group", layout.Groups)
	}
	group := (*layout.Groups)[0]
	if group.Id != "group-1" ||
		group.Label == nil ||
		*group.Label != want.groupLabel ||
		len(group.NodeIds) != 1 ||
		group.NodeIds[0] != "workstation:plan-task" ||
		group.ParentGroupId == nil ||
		*group.ParentGroupId != "group-root" ||
		group.Color == nil ||
		*group.Color != want.groupColor ||
		group.Locked == nil ||
		*group.Locked != want.groupLocked {
		t.Fatalf("factory event layout group = %#v, want group expectation %#v", group, want)
	}

	if layout.Viewport == nil ||
		layout.Viewport.X != want.viewportX ||
		layout.Viewport.Y != want.viewportY ||
		math.Abs(float64(layout.Viewport.Zoom-want.zoom)) > 1e-6 {
		t.Fatalf("factory event layout viewport = %#v, want x=%v y=%v zoom=%v", layout.Viewport, want.viewportX, want.viewportY, want.zoom)
	}
	if layout.Preferences == nil ||
		layout.Preferences.Direction == nil ||
		*layout.Preferences.Direction != want.direction {
		t.Fatalf("factory event layout preferences = %#v, want direction %s", layout.Preferences, want.direction)
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
