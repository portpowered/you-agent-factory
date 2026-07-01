package workgraph

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestDeriveFromJSON_ThreeWorksTwoDependencies(t *testing.T) {
	data := []byte(`{
  "requestId": "graph-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`)

	graph, err := DeriveFromJSON(data)
	if err != nil {
		t.Fatalf("DeriveFromJSON: %v", err)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("node count = %d, want 3", len(graph.Nodes))
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("edge count = %d, want 2", len(graph.Edges))
	}

	wantEdges := []Edge{
		{SourceID: "beta", TargetID: "alpha", Type: "DEPENDS_ON"},
		{SourceID: "gamma", TargetID: "beta", Type: "DEPENDS_ON"},
	}
	if !reflect.DeepEqual(graph.Edges, wantEdges) {
		t.Fatalf("edges = %#v, want %#v", graph.Edges, wantEdges)
	}
}

func TestDeriveFromJSON_StandaloneWorkPreserved(t *testing.T) {
	data := []byte(`{
  "requestId": "standalone-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "solo-a", "workTypeName": "task"},
    {"name": "solo-b", "workTypeName": "task"}
  ]
}`)

	graph, err := DeriveFromJSON(data)
	if err != nil {
		t.Fatalf("DeriveFromJSON: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("node count = %d, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("edge count = %d, want 0", len(graph.Edges))
	}
	wantNodes := []Node{
		{ID: "solo-a", Label: "solo-a"},
		{ID: "solo-b", Label: "solo-b"},
	}
	if !reflect.DeepEqual(graph.Nodes, wantNodes) {
		t.Fatalf("nodes = %#v, want %#v", graph.Nodes, wantNodes)
	}
}

func TestDeriveFromJSON_DeterministicAcrossRuns(t *testing.T) {
	data := []byte(`{
  "requestId": "deterministic-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "z-last", "workTypeName": "task"},
    {"name": "a-first", "workTypeName": "task"},
    {"name": "m-middle", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "m-middle", "targetWorkName": "a-first"},
    {"type": "DEPENDS_ON", "sourceWorkName": "z-last", "targetWorkName": "m-middle"}
  ]
}`)

	first, err := DeriveFromJSON(data)
	if err != nil {
		t.Fatalf("first DeriveFromJSON: %v", err)
	}
	second, err := DeriveFromJSON(data)
	if err != nil {
		t.Fatalf("second DeriveFromJSON: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("graphs differ across runs:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestDeriveFromJSON_NodeLabelsPreferWorkNames(t *testing.T) {
	req := interfaces.WorkRequest{
		RequestID: "label-test",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "Customer Task", WorkTypeID: "task"},
		},
	}

	graph, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromWorkRequest: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(graph.Nodes))
	}
	if graph.Nodes[0].Label != "Customer Task" {
		t.Fatalf("label = %q, want %q", graph.Nodes[0].Label, "Customer Task")
	}
}

func TestNodeLabelFallbackUsesStableIndex(t *testing.T) {
	work := interfaces.Work{WorkID: "batch-request-work-1"}
	if got := nodeLabel(work, 0); got != "batch-request-work-1" {
		t.Fatalf("nodeLabel with workId = %q, want batch-request-work-1", got)
	}
	if got := nodeLabel(interfaces.Work{}, 2); got != "work-3" {
		t.Fatalf("nodeLabel without name/workId = %q, want work-3", got)
	}
}

func TestDeriveFromJSON_InvalidJSON(t *testing.T) {
	_, err := DeriveFromJSON([]byte(`{not json`))
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestDeriveFromJSON_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "missing requestId",
			data: `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}]}`,
			want: "batch requestId is required",
		},
		{
			name: "missing works",
			data: `{"requestId":"x","type":"FACTORY_REQUEST_BATCH","works":[]}`,
			want: "batch works must contain at least one item",
		},
		{
			name: "missing work name",
			data: `{"requestId":"x","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task"}]}`,
			want: "works[0] is missing required name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DeriveFromJSON([]byte(tc.data))
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDeriveFromJSON_UnknownDependencyReference(t *testing.T) {
	data := []byte(`{
  "requestId": "unknown-ref",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "missing"}
  ]
}`)

	_, err := DeriveFromJSON(data)
	if err == nil {
		t.Fatal("expected unknown targetWorkName error")
	}
	if got := err.Error(); got != `relations[0] references unknown targetWorkName "missing"` {
		t.Fatalf("error = %q", got)
	}
}

func TestDeriveFromJSON_RejectsRetiredAliases(t *testing.T) {
	data := []byte(`{
  "requestId": "alias-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "work_type_id": "task"}
  ]
}`)
	_, err := DeriveFromJSON(data)
	if err == nil {
		t.Fatal("expected retired alias error")
	}
	if got := err.Error(); !strings.Contains(got, "work_type_id is not supported") {
		t.Fatalf("error = %q", got)
	}
}

func TestDeriveFromJSON_ParentChildRelationIncluded(t *testing.T) {
	data := []byte(`{
  "requestId": "parent-child",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "child", "workTypeName": "task"},
    {"name": "parent", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"}
  ]
}`)
	graph, err := DeriveFromJSON(data)
	if err != nil {
		t.Fatalf("DeriveFromJSON: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edge count = %d, want 1", len(graph.Edges))
	}
	if graph.Edges[0].Type != "PARENT_CHILD" {
		t.Fatalf("edge type = %q, want PARENT_CHILD", graph.Edges[0].Type)
	}
}

func TestDeriveFromJSON_EdgesSortedDeterministically(t *testing.T) {
	req := interfaces.WorkRequest{
		RequestID: "edge-order",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "a", WorkTypeID: "task"},
			{Name: "b", WorkTypeID: "task"},
			{Name: "c", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{
			{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "c", TargetWorkName: "b"},
			{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "b", TargetWorkName: "a"},
		},
	}

	graph, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromWorkRequest: %v", err)
	}
	raw, err := json.Marshal(graph.Edges)
	if err != nil {
		t.Fatalf("marshal edges: %v", err)
	}
	want := `[{"SourceID":"b","TargetID":"a","Type":"DEPENDS_ON"},{"SourceID":"c","TargetID":"b","Type":"DEPENDS_ON"}]`
	if string(raw) != want {
		t.Fatalf("edges JSON = %s, want %s", raw, want)
	}
}
