package workflowvalidation

import "fmt"

// Issue is one workflow source validation diagnostic.
type Issue struct {
	Code    string
	Message string
	Line    int
	Column  int
	Path    string
}

// LocationSuffix returns a source-location suffix for messages when available.
func (i Issue) LocationSuffix() string {
	if i.Line <= 0 {
		return ""
	}
	if i.Column > 0 {
		return fmt.Sprintf(" (line %d, column %d)", i.Line, i.Column)
	}
	return fmt.Sprintf(" (line %d)", i.Line)
}

// Result aggregates workflow source validation issues.
type Result struct {
	Issues []Issue
}

// HasIssues reports whether validation found one or more issues.
func (r Result) HasIssues() bool {
	return len(r.Issues) > 0
}
