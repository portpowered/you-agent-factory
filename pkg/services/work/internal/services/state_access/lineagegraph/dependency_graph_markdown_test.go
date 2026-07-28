package lineagegraph

import (
	"strings"
	"testing"
)

func TestRenderMarkdownMermaid_IncludesTitleSummaryAndFencedDiagram(t *testing.T) {
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

	got := RenderMarkdownMermaid(graph)
	if !strings.HasPrefix(got, "# Work Dependency Graph\n\n") {
		t.Fatalf("output missing title:\n%s", got)
	}
	if !strings.Contains(got, "3 work items and 2 declared dependencies.") {
		t.Fatalf("output missing summary:\n%s", got)
	}

	start := strings.Index(got, "```mermaid\n")
	end := strings.Index(got, "\n```\n")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("output missing fenced mermaid block:\n%s", got)
	}

	embedded := got[start+len("```mermaid\n") : end]
	raw := strings.TrimSuffix(RenderMermaidFlowchart(graph), "\n")
	if embedded != raw {
		t.Fatalf("embedded mermaid differs from raw output:\nembedded=%q\nraw=%q", embedded, raw)
	}
}

func TestRenderMarkdownMermaid_StandaloneWorkSummary(t *testing.T) {
	graph := Graph{
		Nodes: []Node{
			{ID: "solo-a", Label: "solo-a"},
			{ID: "solo-b", Label: "solo-b"},
		},
	}

	got := RenderMarkdownMermaid(graph)
	if !strings.Contains(got, "2 work items with no declared dependencies.") {
		t.Fatalf("output missing standalone summary:\n%s", got)
	}
	if !strings.Contains(got, `"solo-a"["solo-a"]`) {
		t.Fatalf("output missing solo-a node:\n%s", got)
	}
}
