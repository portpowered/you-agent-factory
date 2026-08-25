package main

import (
	"fmt"
	"strings"
)

func writeCoverageDiagnostics(result coverageResult) {
	for _, warning := range coverageDiagnosticsForOutput(result.detailedDiagnostics, result.packageMinimumWarnings) {
		fmt.Fprintln(stderrWriter, warning)
	}
	for _, warning := range coverageDiagnosticsForOutput(result.detailedDiagnostics, result.manifestCompletenessWarnings) {
		fmt.Fprintln(stderrWriter, warning)
	}
	for _, diagnostic := range coverageDiagnosticsForOutput(result.detailedDiagnostics, result.unmeasuredPackageDiagnostics) {
		fmt.Fprintln(stderrWriter, diagnostic)
	}
}

// coverageDiagnosticsForOutput keeps the evaluated finding intact while
// making the default human-facing contract one line per finding. The optional
// source-block detail is useful when investigating a regression, but it is
// too noisy for ordinary stderr, JSON-backed reports, and verdict handoffs.
func coverageDiagnosticsForOutput(detailed bool, groups ...[]string) []string {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	diagnostics := make([]string, 0, count)
	for _, group := range groups {
		for _, diagnostic := range group {
			if detailed {
				diagnostics = append(diagnostics, diagnostic)
				continue
			}
			diagnostics = append(diagnostics, compactCoverageDiagnostic(diagnostic))
		}
	}
	return diagnostics
}

func compactCoverageDiagnostic(diagnostic string) string {
	const uncoveredBlocksMarker = "; uncovered blocks:"
	for {
		markerIndex := strings.Index(diagnostic, uncoveredBlocksMarker)
		if markerIndex < 0 {
			return strings.TrimSpace(diagnostic)
		}
		tail := diagnostic[markerIndex+len(uncoveredBlocksMarker):]
		if nextField := strings.Index(tail, "; "); nextField >= 0 {
			diagnostic = diagnostic[:markerIndex] + tail[nextField:]
			continue
		}
		return strings.TrimSpace(diagnostic[:markerIndex])
	}
}
