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

// goTestTimingEvent mirrors the subset of the `go test -json` TestEvent
// schema needed for package-level timing. Package-level terminal events carry
// an empty Test field.
type goTestTimingEvent struct {
	Action  string  `json:"Action"`
	Elapsed float64 `json:"Elapsed"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
}

// functionalPackageTimingJSON is one runnable functional-test package's
// elapsed duration and terminal outcome from a single go test -json run.
type functionalPackageTimingJSON struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
	Outcome string  `json:"outcome"`
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
	PackageCount             int                           `json:"packageCount"`
	Packages                 []functionalPackageTimingJSON `json:"packages"`
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
	outcomes := make(map[string]functionalPackageTimingJSON, len(expectedPackages))
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
		if event.Test != "" || strings.TrimSpace(event.Package) == "" {
			continue
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
		if _, exists := outcomes[event.Package]; exists {
			complete = false
			continue
		}
		outcomes[event.Package] = functionalPackageTimingJSON{
			Package: event.Package,
			Seconds: event.Elapsed,
			Outcome: event.Action,
		}
	}
	if err := scanner.Err(); err != nil {
		complete = false
	}

	packages := make([]functionalPackageTimingJSON, 0, len(expectedPackages))
	sum := 0.0
	for _, importPath := range expectedPackages {
		entry, ok := outcomes[importPath]
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

	if wallSeconds < 0 || math.IsNaN(wallSeconds) || math.IsInf(wallSeconds, 0) {
		wallSeconds = 0
		complete = false
	}

	return functionalTimingSummaryJSON{
		Version:                  functionalTimingSummaryVersion,
		Complete:                 complete,
		WallSeconds:              roundTimingSeconds(wallSeconds),
		PackageElapsedSecondsSum: roundTimingSeconds(sum),
		PackageCount:             len(packages),
		Packages:                 packages,
	}
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
