package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const unitTimingSummaryVersion = 1

const (
	unitTimingOutcomePass = "pass"
	unitTimingOutcomeFail = "fail"
	unitTimingOutcomeSkip = "skip"

	unitCacheExecuted = "executed"
	unitCacheCached   = "cached"
	unitCacheUnknown  = "unknown"
)

// goTestUnitTimingEvent mirrors the package-level portion of go test's JSON
// event stream. Package terminal events have an empty Test field; Output
// events are replayed so -json does not make the unit lane's normal output
// unreadable.
type goTestUnitTimingEvent struct {
	Action  string  `json:"Action"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
}

type unitPackageTiming struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
	Outcome string  `json:"outcome"`
	Cache   string  `json:"cache"`
}

// unitTimingSummary is the versioned machine-readable shape emitted by
// unitlane. PackageElapsedSecondsSum is expected to exceed WallSeconds when
// independent packages overlap under go test's package scheduler.
type unitTimingSummary struct {
	Version                  int                 `json:"version"`
	Complete                 bool                `json:"complete"`
	WallSeconds              float64             `json:"wallSeconds"`
	PackageElapsedSecondsSum float64             `json:"packageElapsedSecondsSum"`
	PackageCount             int                 `json:"packageCount"`
	ExpectedPackageCount     int                 `json:"expectedPackageCount"`
	Packages                 []unitPackageTiming `json:"packages"`
}

type unitTimingCapture struct {
	Complete bool
	Packages []unitPackageTiming
}

type unitTimingAccumulator struct {
	complete bool
	expected []string
	packages map[string]unitPackageTiming
}

type unitTimingEventState struct {
	expected       map[string]struct{}
	terminals      map[string]unitPackageTiming
	cacheEvidence  map[string]string
	outputEvidence map[string]bool
	complete       bool
	outputErr      error
}

func newUnitTimingAccumulator(expected []string) *unitTimingAccumulator {
	accumulator := &unitTimingAccumulator{
		complete: true,
		expected: append([]string(nil), expected...),
		packages: make(map[string]unitPackageTiming, len(expected)),
	}
	seen := make(map[string]struct{}, len(expected))
	for _, packageName := range expected {
		if _, exists := seen[packageName]; exists {
			accumulator.complete = false
		}
		seen[packageName] = struct{}{}
	}
	return accumulator
}

func (accumulator *unitTimingAccumulator) add(capture unitTimingCapture) {
	accumulator.complete = accumulator.complete && capture.Complete
	for _, packageTiming := range capture.Packages {
		if _, exists := accumulator.packages[packageTiming.Package]; exists {
			accumulator.complete = false
			continue
		}
		accumulator.packages[packageTiming.Package] = packageTiming
	}
}

func (accumulator *unitTimingAccumulator) summary(wallSeconds float64) unitTimingSummary {
	packages := make([]unitPackageTiming, 0, len(accumulator.packages))
	packageSum := 0.0
	for _, expectedPackage := range accumulator.expected {
		packageTiming, exists := accumulator.packages[expectedPackage]
		if !exists {
			accumulator.complete = false
			continue
		}
		packages = append(packages, packageTiming)
		packageSum += packageTiming.Seconds
	}
	if len(accumulator.packages) != len(packages) {
		accumulator.complete = false
	}
	rankUnitPackageTimings(packages)
	return unitTimingSummary{
		Version:                  unitTimingSummaryVersion,
		Complete:                 accumulator.complete,
		WallSeconds:              roundUnitTimingSeconds(wallSeconds),
		PackageElapsedSecondsSum: roundUnitTimingSeconds(packageSum),
		PackageCount:             len(packages),
		ExpectedPackageCount:     len(accumulator.expected),
		Packages:                 packages,
	}
}

// collectUnitTimingCapture keeps partial package evidence after malformed or
// truncated event data. Parser problems make Complete false, but they do not
// turn an otherwise passing test command into a false test failure. A writer
// failure is returned because it means normal test output was not delivered.
func collectUnitTimingCapture(reader io.Reader, expectedPackages []string, output io.Writer) (unitTimingCapture, error) {
	state := newUnitTimingEventState(expectedPackages)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		state.consume(line, output)
	}
	if scanner.Err() != nil {
		state.complete = false
	}
	return state.capture(expectedPackages), state.outputErr
}

func newUnitTimingEventState(expectedPackages []string) *unitTimingEventState {
	expected := make(map[string]struct{}, len(expectedPackages))
	for _, packageName := range expectedPackages {
		expected[packageName] = struct{}{}
	}
	return &unitTimingEventState{
		expected:       expected,
		terminals:      make(map[string]unitPackageTiming, len(expectedPackages)),
		cacheEvidence:  make(map[string]string, len(expectedPackages)),
		outputEvidence: make(map[string]bool, len(expectedPackages)),
		complete:       true,
	}
}

func (state *unitTimingEventState) consume(line []byte, output io.Writer) {
	var event goTestUnitTimingEvent
	if err := json.Unmarshal(line, &event); err != nil {
		state.complete = false
		state.outputErr = errors.Join(state.outputErr, writeUnitCapturedOutput(output, append(append([]byte(nil), line...), '\n')))
		return
	}
	if event.Output != "" {
		if event.Package != "" {
			state.outputEvidence[event.Package] = true
		}
		if event.Test == "" && strings.Contains(event.Output, "(cached)") && event.Package != "" {
			state.cacheEvidence[event.Package] = unitCacheCached
		}
		state.outputErr = errors.Join(state.outputErr, writeUnitCapturedOutput(output, []byte(event.Output)))
	}
	if event.Test == "" && isUnitTimingTerminal(event.Action) {
		state.recordTerminal(event)
	}
}

func (state *unitTimingEventState) recordTerminal(event goTestUnitTimingEvent) {
	if strings.TrimSpace(event.Package) == "" {
		state.complete = false
		return
	}
	if _, exists := state.expected[event.Package]; !exists {
		state.complete = false
		return
	}
	if math.IsNaN(event.Elapsed) || math.IsInf(event.Elapsed, 0) || event.Elapsed < 0 {
		state.complete = false
		return
	}
	if _, exists := state.terminals[event.Package]; exists {
		state.complete = false
		return
	}
	cacheStatus := state.cacheEvidence[event.Package]
	if cacheStatus == "" {
		cacheStatus = unitCacheExecuted
		if !state.outputEvidence[event.Package] {
			cacheStatus = unitCacheUnknown
		}
	}
	state.terminals[event.Package] = unitPackageTiming{
		Package: event.Package,
		Seconds: event.Elapsed,
		Outcome: event.Action,
		Cache:   cacheStatus,
	}
}

func (state *unitTimingEventState) capture(expectedPackages []string) unitTimingCapture {
	for packageName, packageTiming := range state.terminals {
		if state.cacheEvidence[packageName] == unitCacheCached {
			packageTiming.Cache = unitCacheCached
		} else if state.outputEvidence[packageName] {
			packageTiming.Cache = unitCacheExecuted
		} else if packageTiming.Cache == "" {
			packageTiming.Cache = unitCacheUnknown
		}
		state.terminals[packageName] = packageTiming
	}
	packages := make([]unitPackageTiming, 0, len(state.terminals))
	for _, packageName := range expectedPackages {
		packageTiming, exists := state.terminals[packageName]
		if !exists {
			state.complete = false
			continue
		}
		packages = append(packages, packageTiming)
	}
	if len(state.terminals) != len(packages) {
		state.complete = false
	}
	return unitTimingCapture{Complete: state.complete, Packages: packages}
}

func isUnitTimingTerminal(action string) bool {
	switch action {
	case unitTimingOutcomePass, unitTimingOutcomeFail, unitTimingOutcomeSkip:
		return true
	default:
		return false
	}
}

func rankUnitPackageTimings(packages []unitPackageTiming) {
	slices.SortFunc(packages, func(left, right unitPackageTiming) int {
		if left.Seconds > right.Seconds {
			return -1
		}
		if left.Seconds < right.Seconds {
			return 1
		}
		return strings.Compare(left.Package, right.Package)
	})
}

func renderUnitTimingSummary(summary unitTimingSummary) string {
	var builder strings.Builder
	builder.WriteString("\nUnit lane timing summary\n")
	if summary.Complete {
		builder.WriteString("Capture: complete\n")
	} else {
		builder.WriteString("Capture: incomplete (partial diagnostics only)\n")
	}
	fmt.Fprintf(&builder, "Total wall time: %.3fs\n", summary.WallSeconds)
	fmt.Fprintf(&builder, "Package elapsed sum: %.3fs\n", summary.PackageElapsedSecondsSum)
	fmt.Fprintf(&builder, "Package count: %d/%d\n", summary.PackageCount, summary.ExpectedPackageCount)
	fmt.Fprintf(&builder, "Cache evidence: %s\n", unitCacheCounts(summary.Packages))
	if len(summary.Packages) == 0 {
		return builder.String()
	}

	builder.WriteString("Rank  Package  Elapsed  Outcome  Cache\n")
	for index, packageTiming := range summary.Packages {
		fmt.Fprintf(
			&builder,
			"%4d  %s  %.3fs  %s  %s\n",
			index+1,
			packageTiming.Package,
			packageTiming.Seconds,
			packageTiming.Outcome,
			packageTiming.Cache,
		)
	}
	return builder.String()
}

func unitCacheCounts(packages []unitPackageTiming) string {
	counts := map[string]int{
		unitCacheCached:   0,
		unitCacheExecuted: 0,
		unitCacheUnknown:  0,
	}
	for _, packageTiming := range packages {
		if _, exists := counts[packageTiming.Cache]; !exists {
			counts[unitCacheUnknown]++
			continue
		}
		counts[packageTiming.Cache]++
	}
	return fmt.Sprintf("%d cached, %d executed, %d unknown", counts[unitCacheCached], counts[unitCacheExecuted], counts[unitCacheUnknown])
}

func writeUnitTimingSummary(output io.Writer, summary unitTimingSummary) error {
	if _, err := io.WriteString(output, renderUnitTimingSummary(summary)); err != nil {
		return fmt.Errorf("write unit timing summary: %w", err)
	}
	return nil
}

func renderUnitTimingSummaryJSON(summary unitTimingSummary) ([]byte, error) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render unit timing summary JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func writeUnitTimingSummaryJSON(path string, summary unitTimingSummary) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if directory := filepath.Dir(path); directory != "." && directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create unit timing output directory: %w", err)
		}
	}
	data, err := renderUnitTimingSummaryJSON(summary)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write unit timing summary JSON: %w", err)
	}
	return nil
}

func roundUnitTimingSeconds(seconds float64) float64 {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return math.Round(seconds*1000) / 1000
}

func writeUnitCapturedOutput(output io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := output.Write(data)
	return err
}
