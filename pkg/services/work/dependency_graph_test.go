package work

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeriveFromWorkRequest_ThreeWorksTwoDependencies(t *testing.T) {
	req := WorkRequest{
		RequestID: "graph-test",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "alpha", WorkTypeID: "task"},
			{Name: "beta", WorkTypeID: "task"},
			{Name: "gamma", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationDependsOn, SourceWorkName: "beta", TargetWorkName: "alpha"},
			{Type: WorkRelationDependsOn, SourceWorkName: "gamma", TargetWorkName: "beta"},
		},
	}

	graph, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromWorkRequest: %v", err)
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

func TestDeriveFromWorkRequest_StandaloneWorkPreserved(t *testing.T) {
	req := WorkRequest{
		RequestID: "standalone-test",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "solo-a", WorkTypeID: "task"},
			{Name: "solo-b", WorkTypeID: "task"},
		},
	}

	graph, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromWorkRequest: %v", err)
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

func TestDeriveFromWorkRequest_DeterministicAcrossRuns(t *testing.T) {
	req := WorkRequest{
		RequestID: "deterministic-test",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "z-last", WorkTypeID: "task"},
			{Name: "a-first", WorkTypeID: "task"},
			{Name: "m-middle", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationDependsOn, SourceWorkName: "m-middle", TargetWorkName: "a-first"},
			{Type: WorkRelationDependsOn, SourceWorkName: "z-last", TargetWorkName: "m-middle"},
		},
	}

	first, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("first DeriveFromWorkRequest: %v", err)
	}
	second, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("second DeriveFromWorkRequest: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("graphs differ across runs:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestDeriveFromWorkRequest_NodeLabelsPreferWorkNames(t *testing.T) {
	req := WorkRequest{
		RequestID: "label-test",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
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
	item := Work{WorkID: "batch-request-work-1"}
	if got := nodeLabel(item, 0); got != "batch-request-work-1" {
		t.Fatalf("nodeLabel with workId = %q, want batch-request-work-1", got)
	}
	if got := nodeLabel(Work{}, 2); got != "work-3" {
		t.Fatalf("nodeLabel without name/workId = %q, want work-3", got)
	}
}

func TestDeriveFromWorkRequest_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		req  WorkRequest
		want string
	}{
		{
			name: "missing requestId",
			req: WorkRequest{Type: WorkRequestTypeFactoryRequestBatch,
				Works: []Work{{Name: "a", WorkTypeID: "task"}}},
			want: "batch requestId is required",
		},
		{
			name: "missing works",
			req:  WorkRequest{RequestID: "x", Type: WorkRequestTypeFactoryRequestBatch},
			want: "batch works must contain at least one item",
		},
		{
			name: "missing work name",
			req: WorkRequest{RequestID: "x", Type: WorkRequestTypeFactoryRequestBatch,
				Works: []Work{{WorkTypeID: "task"}}},
			want: "works[0] is missing required name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DeriveFromWorkRequest(tc.req)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDeriveFromWorkRequest_UnknownDependencyReference(t *testing.T) {
	req := WorkRequest{
		RequestID: "unknown-ref",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "alpha", WorkTypeID: "task"}},
		Relations: []WorkRelation{{
			Type: WorkRelationDependsOn, SourceWorkName: "alpha", TargetWorkName: "missing",
		}},
	}

	_, err := DeriveFromWorkRequest(req)
	if err == nil {
		t.Fatal("expected unknown targetWorkName error")
	}
	if got := err.Error(); got != `relations[0] references unknown targetWorkName "missing"` {
		t.Fatalf("error = %q", got)
	}
}

func TestDeriveFromWorkRequest_ParentChildRelationIncluded(t *testing.T) {
	req := WorkRequest{
		RequestID: "parent-child",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "child", WorkTypeID: "task"},
			{Name: "parent", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{{
			Type: WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent",
		}},
	}
	graph, err := DeriveFromWorkRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromWorkRequest: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edge count = %d, want 1", len(graph.Edges))
	}
	if graph.Edges[0].Type != "PARENT_CHILD" {
		t.Fatalf("edge type = %q, want PARENT_CHILD", graph.Edges[0].Type)
	}
}

func TestDeriveFromWorkRequest_EdgesSortedDeterministically(t *testing.T) {
	req := WorkRequest{
		RequestID: "edge-order",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "a", WorkTypeID: "task"},
			{Name: "b", WorkTypeID: "task"},
			{Name: "c", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationDependsOn, SourceWorkName: "c", TargetWorkName: "b"},
			{Type: WorkRelationDependsOn, SourceWorkName: "b", TargetWorkName: "a"},
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
