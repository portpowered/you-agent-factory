package main

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// functionalTimingSummaryVersion is the schema version for the timing summary
// JSON written by gocoveragecheck. Bump this when the document shape changes
// in a way downstream consumers must branch on.
const functionalTimingSummaryVersion = 1

const (
	timingOutcomePass = "pass"
	timingOutcomeFail = "fail"
	timingOutcomeSkip = "skip"
)

const (
	functionalPackageStateCompleted  = "completed"
	functionalPackageStateInFlight   = "in_flight"
	functionalPackageStateUnobserved = "unobserved"
)

const maxTimingFailureReasonLength = 240

// goTestTimingEvent mirrors the subset of the `go test -json` TestEvent
// schema needed for package-level timing. Package-level terminal events carry
// an empty Test field.
type goTestTimingEvent struct {
	Action  string  `json:"Action"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
}

// functionalPackageTimingJSON is one runnable functional-test package's
// elapsed duration and terminal outcome from a single go test -json run.
type functionalPackageTimingJSON struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
	Outcome string  `json:"outcome"`
	Reason  string  `json:"reason,omitempty"`
}

// functionalPackageStateJSON keeps the package-level state visible while a
// functional run is still active. Packages with a terminal go test event also
// appear in Packages; in-flight and unobserved packages are retained here so a
// timeout snapshot names the work that did not finish.
type functionalPackageStateJSON struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
	State   string  `json:"state"`
	Outcome string  `json:"outcome,omitempty"`
	Reason  string  `json:"reason,omitempty"`
}

// functionalTestTimingJSON is one top-level Go test's elapsed duration and
// terminal outcome from the same go test -json run as the package timings.
// Subtests are intentionally excluded so selectors can be mapped directly to
// the top-level tests that contributors see in the functional inventory.
type functionalTestTimingJSON struct {
	Package string  `json:"package"`
	Test    string  `json:"test"`
	Seconds float64 `json:"seconds"`
	Outcome string  `json:"outcome"`
	Reason  string  `json:"reason,omitempty"`
}

// functionalTimingSummaryJSON is the machine-readable timing summary owned by
// gocoveragecheck. WallSeconds is measured around the single go test
// invocation; PackageElapsedSecondsSum is the sum of per-package elapsed
// durations and can exceed WallSeconds because packages run concurrently.
// Complete is false whenever any runnable package never reported a terminal
// event or the event stream was malformed, so downstream consumers can
// distinguish trustworthy partial diagnostics from a fully captured run.
type functionalTimingSummaryJSON struct {
	Version                  int                           `json:"version"`
	Complete                 bool                          `json:"complete"`
	CaptureReason            string                        `json:"captureReason,omitempty"`
	WallSeconds              float64                       `json:"wallSeconds"`
	PackageElapsedSecondsSum float64                       `json:"packageElapsedSecondsSum"`
	ExpectedPackageCount     int                           `json:"expectedPackageCount"`
	PackageCount             int                           `json:"packageCount"`
	TestCount                int                           `json:"testCount"`
	TestPassCount            int                           `json:"testPassCount"`
	TestFailCount            int                           `json:"testFailCount"`
	TestSkipCount            int                           `json:"testSkipCount"`
	Packages                 []functionalPackageTimingJSON `json:"packages"`
	PackageStates            []functionalPackageStateJSON  `json:"packageStates,omitempty"`
	Tests                    []functionalTestTimingJSON    `json:"tests"`
}

// buildFunctionalTimingSummary parses newline-delimited `go test -json`
// events captured from a single test invocation and reports exactly one
// terminal (pass/fail/skip) timing record per expected package, in
// deterministic package-path order. It never errors: malformed JSON lines,
// out-of-range elapsed values, duplicate terminal events, or packages that
// never reported a terminal event all mark the summary incomplete instead of
// aborting capture, so partial diagnostics from a crashed or truncated run
// stay inspectable.
func buildFunctionalTimingSummary(jsonOutput string, expectedPackages []string, wallSeconds float64) functionalTimingSummaryJSON {
	parsed := parseFunctionalTimingEvents(jsonOutput, expectedPackages)
	complete := parsed.complete
	packages, sum := collectFunctionalPackageTimings(parsed.packageOutcomes, expectedPackages, &complete)
	slices.SortFunc(packages, func(left, right functionalPackageTimingJSON) int {
		return strings.Compare(left.Package, right.Package)
	})
	tests := collectFunctionalTestTimings(parsed.testOutcomes, parsed.failureReasons)
	slices.SortFunc(tests, func(left, right functionalTestTimingJSON) int {
		if result := strings.Compare(left.Package, right.Package); result != 0 {
			return result
		}
		return strings.Compare(left.Test, right.Test)
	})

	testPassCount, testFailCount, testSkipCount := countTimingTestOutcomes(tests)

	if wallSeconds < 0 || math.IsNaN(wallSeconds) || math.IsInf(wallSeconds, 0) {
		wallSeconds = 0
		complete = false
	}

	return functionalTimingSummaryJSON{
		Version:                  functionalTimingSummaryVersion,
		Complete:                 complete,
		WallSeconds:              roundTimingSeconds(wallSeconds),
		PackageElapsedSecondsSum: roundTimingSeconds(sum),
		ExpectedPackageCount:     len(expectedPackages),
		PackageCount:             len(packages),
		TestCount:                len(tests),
		TestPassCount:            testPassCount,
		TestFailCount:            testFailCount,
		TestSkipCount:            testSkipCount,
		Packages:                 packages,
		Tests:                    tests,
	}
}

func collectFunctionalPackageTimings(outcomes map[string]functionalPackageTimingJSON, expectedPackages []string, complete *bool) ([]functionalPackageTimingJSON, float64) {
	packages := make([]functionalPackageTimingJSON, 0, len(expectedPackages))
	sum := 0.0
	for _, importPath := range expectedPackages {
		entry, ok := outcomes[importPath]
		if !ok {
			*complete = false
			continue
		}
		packages = append(packages, entry)
		sum += entry.Seconds
	}
	return packages, sum
}

func collectFunctionalTestTimings(outcomes map[string]functionalTestTimingJSON, failureReasons map[string]string) []functionalTestTimingJSON {
	tests := make([]functionalTestTimingJSON, 0, len(outcomes))
	for _, entry := range outcomes {
		if entry.Reason == "" {
			entry.Reason = failureReasons[timingEventKey(entry.Package, entry.Test)]
		}
		tests = append(tests, entry)
	}
	return tests
}

func timingEventKey(importPath, testName string) string {
	return importPath + "\x00" + testName
}

func firstTimingFailureReason(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=== RUN") || strings.HasPrefix(line, "=== PAUSE") || strings.HasPrefix(line, "=== CONT") {
			continue
		}
		if len(line) > maxTimingFailureReasonLength {
			return line[:maxTimingFailureReasonLength] + "..."
		}
		return line
	}
	return ""
}

func countTimingTestOutcomes(tests []functionalTestTimingJSON) (pass, fail, skip int) {
	for _, test := range tests {
		switch test.Outcome {
		case timingOutcomePass:
			pass++
		case timingOutcomeFail:
			fail++
		case timingOutcomeSkip:
			skip++
		}
	}
	return pass, fail, skip
}

func writeFunctionalTimingInventorySummary(summary functionalTimingSummaryJSON, short bool) {
	packagePassCount, packageFailCount, packageSkipCount := 0, 0, 0
	for _, pkg := range summary.Packages {
		switch pkg.Outcome {
		case timingOutcomePass:
			packagePassCount++
		case timingOutcomeFail:
			packageFailCount++
		case timingOutcomeSkip:
			packageSkipCount++
		}
	}
	deferredShortTests := 0
	if short {
		// The short tier deliberately leaves tests guarded by testing.Short in
		// the discovered selection. Their skip outcomes are deferred work, not
		// quarantine: the complete merge tier runs them with -short=false.
		deferredShortTests = summary.TestSkipCount
	}
	fmt.Fprintf(
		stdoutWriter,
		"Functional suite inventory: discovered-packages=%d observed-packages=%d (pass=%d fail=%d skip=%d) top-level-tests=%d (pass=%d fail=%d skip=%d) deferred-short-tests=%d wall=%.3fs complete=%t\n",
		summary.ExpectedPackageCount,
		summary.PackageCount,
		packagePassCount,
		packageFailCount,
		packageSkipCount,
		summary.TestCount,
		summary.TestPassCount,
		summary.TestFailCount,
		summary.TestSkipCount,
		deferredShortTests,
		summary.WallSeconds,
		summary.Complete,
	)
}

func roundTimingSeconds(seconds float64) float64 {
	return math.Round(seconds*1000) / 1000
}

func renderFunctionalTimingSummaryJSON(summary functionalTimingSummaryJSON) ([]byte, error) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render go functional timing summary json: %w", err)
	}
	return append(data, '\n'), nil
}

func writeFunctionalTimingSummaryJSON(path string, summary functionalTimingSummaryJSON) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	data, err := renderFunctionalTimingSummaryJSON(summary)
	if err != nil {
		return err
	}
	if err := writeAtomicDiagnosticFile(path, data); err != nil {
		return fmt.Errorf("write go functional timing summary json: %w", err)
	}
	return nil
}
