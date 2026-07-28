package lineagegraph

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeriveFromBatchRequest_ThreeWorksTwoDependencies(t *testing.T) {
	req := BatchRequest{
		RequestID: "graph-test",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "alpha", WorkTypeID: "task"},
			{Name: "beta", WorkTypeID: "task"},
			{Name: "gamma", WorkTypeID: "task"},
		},
		Relations: []BatchRelation{
			{Type: RelationDependsOn, SourceWorkName: "beta", TargetWorkName: "alpha"},
			{Type: RelationDependsOn, SourceWorkName: "gamma", TargetWorkName: "beta"},
		},
	}

	graph, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromBatchRequest: %v", err)
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

func TestDeriveFromBatchRequest_StandaloneWorkPreserved(t *testing.T) {
	req := BatchRequest{
		RequestID: "standalone-test",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "solo-a", WorkTypeID: "task"},
			{Name: "solo-b", WorkTypeID: "task"},
		},
	}

	graph, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromBatchRequest: %v", err)
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

func TestDeriveFromBatchRequest_DeterministicAcrossRuns(t *testing.T) {
	req := BatchRequest{
		RequestID: "deterministic-test",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "z-last", WorkTypeID: "task"},
			{Name: "a-first", WorkTypeID: "task"},
			{Name: "m-middle", WorkTypeID: "task"},
		},
		Relations: []BatchRelation{
			{Type: RelationDependsOn, SourceWorkName: "m-middle", TargetWorkName: "a-first"},
			{Type: RelationDependsOn, SourceWorkName: "z-last", TargetWorkName: "m-middle"},
		},
	}

	first, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("first DeriveFromBatchRequest: %v", err)
	}
	second, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("second DeriveFromBatchRequest: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("graphs differ across runs:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestDeriveFromBatchRequest_NodeLabelsPreferWorkNames(t *testing.T) {
	req := BatchRequest{
		RequestID: "label-test",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "Customer Task", WorkTypeID: "task"},
		},
	}

	graph, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromBatchRequest: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(graph.Nodes))
	}
	if graph.Nodes[0].Label != "Customer Task" {
		t.Fatalf("label = %q, want %q", graph.Nodes[0].Label, "Customer Task")
	}
}

func TestNodeLabelFallbackUsesStableIndex(t *testing.T) {
	item := BatchWork{WorkID: "batch-request-work-1"}
	if got := nodeLabel(item, 0); got != "batch-request-work-1" {
		t.Fatalf("nodeLabel with workId = %q, want batch-request-work-1", got)
	}
	if got := nodeLabel(BatchWork{}, 2); got != "work-3" {
		t.Fatalf("nodeLabel without name/workId = %q, want work-3", got)
	}
}

func TestDeriveFromBatchRequest_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		req  BatchRequest
		want string
	}{
		{
			name: "missing requestId",
			req: BatchRequest{Type: BatchRequestTypeFactoryRequestBatch,
				Works: []BatchWork{{Name: "a", WorkTypeID: "task"}}},
			want: "batch requestId is required",
		},
		{
			name: "missing works",
			req:  BatchRequest{RequestID: "x", Type: BatchRequestTypeFactoryRequestBatch},
			want: "batch works must contain at least one item",
		},
		{
			name: "missing work name",
			req: BatchRequest{RequestID: "x", Type: BatchRequestTypeFactoryRequestBatch,
				Works: []BatchWork{{WorkTypeID: "task"}}},
			want: "works[0] is missing required name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DeriveFromBatchRequest(tc.req)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDeriveFromBatchRequest_UnknownDependencyReference(t *testing.T) {
	req := BatchRequest{
		RequestID: "unknown-ref",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works:     []BatchWork{{Name: "alpha", WorkTypeID: "task"}},
		Relations: []BatchRelation{{
			Type: RelationDependsOn, SourceWorkName: "alpha", TargetWorkName: "missing",
		}},
	}

	_, err := DeriveFromBatchRequest(req)
	if err == nil {
		t.Fatal("expected unknown targetWorkName error")
	}
	if got := err.Error(); got != `relations[0] references unknown targetWorkName "missing"` {
		t.Fatalf("error = %q", got)
	}
}

func TestDeriveFromBatchRequest_ParentChildRelationIncluded(t *testing.T) {
	req := BatchRequest{
		RequestID: "parent-child",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "child", WorkTypeID: "task"},
			{Name: "parent", WorkTypeID: "task"},
		},
		Relations: []BatchRelation{{
			Type: RelationParentChild, SourceWorkName: "child", TargetWorkName: "parent",
		}},
	}
	graph, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromBatchRequest: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edge count = %d, want 1", len(graph.Edges))
	}
	if graph.Edges[0].Type != "PARENT_CHILD" {
		t.Fatalf("edge type = %q, want PARENT_CHILD", graph.Edges[0].Type)
	}
}

func TestDeriveFromBatchRequest_EdgesSortedDeterministically(t *testing.T) {
	req := BatchRequest{
		RequestID: "edge-order",
		Type:      BatchRequestTypeFactoryRequestBatch,
		Works: []BatchWork{
			{Name: "a", WorkTypeID: "task"},
			{Name: "b", WorkTypeID: "task"},
			{Name: "c", WorkTypeID: "task"},
		},
		Relations: []BatchRelation{
			{Type: RelationDependsOn, SourceWorkName: "c", TargetWorkName: "b"},
			{Type: RelationDependsOn, SourceWorkName: "b", TargetWorkName: "a"},
		},
	}

	graph, err := DeriveFromBatchRequest(req)
	if err != nil {
		t.Fatalf("DeriveFromBatchRequest: %v", err)
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
