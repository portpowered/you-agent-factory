package work

import (
	"fmt"
	"strings"
)

// RenderMarkdownMermaid renders a Markdown document with a title, short summary,
// and one fenced Mermaid flowchart for the same graph as RenderMermaidFlowchart.
func RenderMarkdownMermaid(g Graph) string {
	var b strings.Builder
	b.WriteString("# Work Dependency Graph\n\n")
	b.WriteString(markdownMermaidSummary(g))
	b.WriteString("\n\n```mermaid\n")
	b.WriteString(RenderMermaidFlowchart(g))
	b.WriteString("```\n")
	return b.String()
}

func markdownMermaidSummary(g Graph) string {
	workCount := len(g.Nodes)
	edgeCount := len(g.Edges)
	switch {
	case workCount == 0:
		return "No work items in batch."
	case edgeCount == 0:
		if workCount == 1 {
			return "1 work item with no declared dependencies."
		}
		return fmt.Sprintf("%d work items with no declared dependencies.", workCount)
	case workCount == 1:
		return fmt.Sprintf("1 work item and %d declared %s.",
			edgeCount, dependencyWord(edgeCount))
	default:
		return fmt.Sprintf("%d work items and %d declared %s.",
			workCount, edgeCount, dependencyWord(edgeCount))
	}
}

func dependencyWord(count int) string {
	if count == 1 {
		return "dependency"
	}
	return "dependencies"
}
