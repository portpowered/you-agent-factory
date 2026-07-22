package work

import (
	"strings"
	"unicode"
)

// RenderMermaidFlowchart renders a deterministic Mermaid flowchart diagram.
func RenderMermaidFlowchart(g Graph) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, node := range g.Nodes {
		b.WriteString("  ")
		b.WriteString(mermaidNodeRef(node.ID))
		b.WriteString("[\"")
		b.WriteString(escapeMermaidLabel(node.Label))
		b.WriteString("\"]\n")
	}
	for _, edge := range g.Edges {
		b.WriteString("  ")
		b.WriteString(mermaidNodeRef(edge.SourceID))
		b.WriteString(" --> ")
		b.WriteString(mermaidNodeRef(edge.TargetID))
		b.WriteString("\n")
	}
	return b.String()
}

func mermaidNodeRef(id string) string {
	if isMermaidBareID(id) {
		return id
	}
	return `"` + escapeMermaidQuotedString(id) + `"`
}

func isMermaidBareID(id string) bool {
	if id == "" {
		return false
	}
	first := rune(id[0])
	if !unicode.IsLetter(first) && first != '_' {
		return false
	}
	for _, r := range id[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func escapeMermaidLabel(label string) string {
	replacer := strings.NewReplacer(
		"#", "#35;",
		`"`, "#quot;",
		"[", "#91;",
		"]", "#93;",
		"(", "#40;",
		")", "#41;",
		";", "#59;",
		"\n", " ",
		"\r", " ",
	)
	return replacer.Replace(label)
}

func escapeMermaidQuotedString(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n', '\r':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
