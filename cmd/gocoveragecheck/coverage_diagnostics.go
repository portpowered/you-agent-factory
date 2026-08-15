package main

import (
	"errors"
	"fmt"
)

func writeCoverageTestFailureWarning(err error) {
	var testFailureErr *coverageTestFailureError
	if !errors.As(err, &testFailureErr) {
		return
	}
	if testFailureErr.failedTestCountKnown {
		fmt.Fprintf(stderrWriter, "coverage not evaluated: %d failed tests observed; package floors were NOT checked because the coverage test run failed\n", testFailureErr.failedTestCount)
		return
	}
	fmt.Fprintln(stderrWriter, "coverage not evaluated: package floors were NOT checked because the coverage test run failed; failed-test count unavailable")
}

func writeCoverageDiagnostics(result coverageResult) {
	for _, warning := range result.packageMinimumWarnings {
		fmt.Fprintln(stderrWriter, warning)
	}
	for _, diagnostic := range result.unmeasuredPackageDiagnostics {
		fmt.Fprintln(stderrWriter, diagnostic)
	}
}
