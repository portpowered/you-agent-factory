package lineagegraph

import (
	"strings"
	"testing"
)

func TestRenderMermaidFlowchart_IncludesAllNodesAndEdges(t *testing.T) {
	graph := Graph{
		Nodes: []Node{
			{ID: "alpha", Label: "alpha"},
			{ID: "beta", Label: "beta"},
			{ID: "gamma", Label: "gamma"},
		},
		Edges: []Edge{
			{SourceID: "beta", TargetID: "alpha", Type: "DEPENDS_ON"},
			{SourceID: "gamma", TargetID: "beta", Type: "DEPENDS_ON"},
		},
	}

	got := RenderMermaidFlowchart(graph)
	if !strings.HasPrefix(got, "flowchart TD\n") {
		t.Fatalf("output missing flowchart header:\n%s", got)
	}
	for _, want := range []string{
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderMermaidFlowchart_StandaloneNodesIncluded(t *testing.T) {
	graph := Graph{
		Nodes: []Node{
			{ID: "solo-a", Label: "solo-a"},
			{ID: "solo-b", Label: "solo-b"},
		},
	}

	got := RenderMermaidFlowchart(graph)
	if strings.Contains(got, "-->") {
		t.Fatalf("standalone graph should not contain edges:\n%s", got)
	}
	for _, want := range []string{
		`"solo-a"["solo-a"]`,
		`"solo-b"["solo-b"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderMermaidFlowchart_EscapesProblematicLabels(t *testing.T) {
	graph := Graph{
		Nodes: []Node{
			{ID: "quoted", Label: `Review "urgent" [now]`},
		},
	}

	got := RenderMermaidFlowchart(graph)
	wantLabel := `quoted["Review #quot;urgent#quot; #91;now#93;"]`
	if !strings.Contains(got, wantLabel) {
		t.Fatalf("output = %q, want substring %q", got, wantLabel)
	}
}

func TestRenderMermaidFlowchart_StableEdgeOrder(t *testing.T) {
	graph := Graph{
		Nodes: []Node{
			{ID: "a", Label: "a"},
			{ID: "b", Label: "b"},
			{ID: "c", Label: "c"},
		},
		Edges: []Edge{
			{SourceID: "c", TargetID: "b", Type: "DEPENDS_ON"},
			{SourceID: "b", TargetID: "a", Type: "DEPENDS_ON"},
		},
	}
	sortEdges(graph.Edges)

	first := RenderMermaidFlowchart(graph)
	second := RenderMermaidFlowchart(graph)
	if first != second {
		t.Fatalf("mermaid output differs across runs:\nfirst=%q\nsecond=%q", first, second)
	}

	bIndex := strings.Index(first, "b --> a")
	cIndex := strings.Index(first, "c --> b")
	if bIndex < 0 || cIndex < 0 || bIndex > cIndex {
		t.Fatalf("edge order unstable:\n%s", first)
	}
}
