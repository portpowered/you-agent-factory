package main

import "fmt"

func writeCoverageDiagnostics(result coverageResult) {
	for _, warning := range result.packageMinimumWarnings {
		fmt.Fprintln(stderrWriter, warning)
	}
	for _, warning := range result.manifestCompletenessWarnings {
		fmt.Fprintln(stderrWriter, warning)
	}
	for _, diagnostic := range result.unmeasuredPackageDiagnostics {
		fmt.Fprintln(stderrWriter, diagnostic)
	}
}
