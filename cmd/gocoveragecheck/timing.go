package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	WallSeconds              float64                       `json:"wallSeconds"`
	PackageElapsedSecondsSum float64                       `json:"packageElapsedSecondsSum"`
	ExpectedPackageCount     int                           `json:"expectedPackageCount"`
	PackageCount             int                           `json:"packageCount"`
	TestCount                int                           `json:"testCount"`
	TestPassCount            int                           `json:"testPassCount"`
	TestFailCount            int                           `json:"testFailCount"`
	TestSkipCount            int                           `json:"testSkipCount"`
	Packages                 []functionalPackageTimingJSON `json:"packages"`
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
	packageOutcomes := make(map[string]functionalPackageTimingJSON, len(expectedPackages))
	testOutcomes := make(map[string]functionalTestTimingJSON)
	failureReasons := make(map[string]string)
	complete := true

	scanner := bufio.NewScanner(strings.NewReader(jsonOutput))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event goTestTimingEvent
		if err := json.Unmarshal(line, &event); err != nil {
			complete = false
			continue
		}
		if strings.TrimSpace(event.Package) == "" {
			continue
		}
		if event.Output != "" {
			key := timingEventKey(event.Package, event.Test)
			if reason := firstTimingFailureReason(event.Output); reason != "" && failureReasons[key] == "" {
				failureReasons[key] = reason
			}
		}
		switch event.Action {
		case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		default:
			continue
		}
		if math.IsNaN(event.Elapsed) || math.IsInf(event.Elapsed, 0) || event.Elapsed < 0 {
			complete = false
			continue
		}
		if event.Test != "" {
			// A slash identifies a Go subtest. The parent top-level test is
			// emitted separately and is the selector we need to inventory.
			if strings.Contains(event.Test, "/") {
				continue
			}
			key := timingEventKey(event.Package, event.Test)
			if _, exists := testOutcomes[key]; exists {
				complete = false
				continue
			}
			testOutcomes[key] = functionalTestTimingJSON{
				Package: event.Package,
				Test:    event.Test,
				Seconds: event.Elapsed,
				Outcome: event.Action,
				Reason:  failureReasons[key],
			}
			continue
		}
		if _, exists := packageOutcomes[event.Package]; exists {
			complete = false
			continue
		}
		key := timingEventKey(event.Package, "")
		packageOutcomes[event.Package] = functionalPackageTimingJSON{
			Package: event.Package,
			Seconds: event.Elapsed,
			Outcome: event.Action,
			Reason:  failureReasons[key],
		}
	}
	if err := scanner.Err(); err != nil {
		complete = false
	}

	packages := make([]functionalPackageTimingJSON, 0, len(expectedPackages))
	sum := 0.0
	for _, importPath := range expectedPackages {
		entry, ok := packageOutcomes[importPath]
		if !ok {
			complete = false
			continue
		}
		packages = append(packages, entry)
		sum += entry.Seconds
	}
	slices.SortFunc(packages, func(left, right functionalPackageTimingJSON) int {
		return strings.Compare(left.Package, right.Package)
	})

	tests := make([]functionalTestTimingJSON, 0, len(testOutcomes))
	for _, entry := range testOutcomes {
		if entry.Reason == "" {
			entry.Reason = failureReasons[timingEventKey(entry.Package, entry.Test)]
		}
		tests = append(tests, entry)
	}
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

func writeFunctionalTimingInventorySummary(summary functionalTimingSummaryJSON) {
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
	fmt.Fprintf(
		stdoutWriter,
		"Functional suite inventory: discovered-packages=%d observed-packages=%d (pass=%d fail=%d skip=%d) top-level-tests=%d (pass=%d fail=%d skip=%d) wall=%.3fs complete=%t\n",
		summary.ExpectedPackageCount,
		summary.PackageCount,
		packagePassCount,
		packageFailCount,
		packageSkipCount,
		summary.TestCount,
		summary.TestPassCount,
		summary.TestFailCount,
		summary.TestSkipCount,
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
	if directory := filepath.Dir(path); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create go functional timing summary directory: %w", err)
		}
	}
	data, err := renderFunctionalTimingSummaryJSON(summary)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write go functional timing summary json: %w", err)
	}
	return nil
}
