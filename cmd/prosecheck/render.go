package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// SortFindings returns a copy in the analyzer's documented total order:
// normalized source path, source line, source column, rule ID, content class,
// structured identity, excerpt, and fingerprint. No map or input order can
// affect the terminal result.
func SortFindings(findings []Finding) []Finding {
	ordered := append([]Finding(nil), findings...)
	slices.SortStableFunc(ordered, func(left, right Finding) int {
		if result := strings.Compare(left.SourcePath, right.SourcePath); result != 0 {
			return result
		}
		if left.StartLine != right.StartLine {
			return left.StartLine - right.StartLine
		}
		if left.StartColumn != right.StartColumn {
			return left.StartColumn - right.StartColumn
		}
		if result := strings.Compare(string(left.RuleID), string(right.RuleID)); result != 0 {
			return result
		}
		if result := strings.Compare(string(left.ContentClass), string(right.ContentClass)); result != 0 {
			return result
		}
		if result := strings.Compare(left.Identity, right.Identity); result != 0 {
			return result
		}
		if result := strings.Compare(left.Excerpt, right.Excerpt); result != 0 {
			return result
		}
		return strings.Compare(left.Fingerprint, right.Fingerprint)
	})
	return ordered
}

// RenderFindings emits one concise, stable diagnostic per line. The output is
// intended for authors; Finding remains the machine-readable contract.
func RenderFindings(writer io.Writer, findings []Finding) error {
	for _, finding := range SortFindings(findings) {
		if _, err := fmt.Fprintf(writer, "%s:%d:%d [%s] %s: %s — %s (%s)\n",
			finding.SourcePath,
			finding.StartLine,
			finding.StartColumn,
			finding.RuleID,
			finding.ContentClass,
			finding.Excerpt,
			finding.Guidance,
			finding.Fingerprint,
		); err != nil {
			return err
		}
	}
	return nil
}
