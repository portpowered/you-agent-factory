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
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const unitTimingSummaryVersion = 2

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
	Package string   `json:"package"`
	Seconds float64  `json:"seconds"`
	Outcome string   `json:"outcome"`
	Cache   string   `json:"cache"`
	Tests   []string `json:"tests"`
}

type unitTimingRunner struct {
	Provider     string `json:"provider"`
	Image        string `json:"image"`
	ImageVersion string `json:"imageVersion"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUModel     string `json:"cpuModel"`
}

type unitTimingRun struct {
	Commit                   string           `json:"commit"`
	GoVersion                string           `json:"goVersion"`
	Command                  string           `json:"command"`
	UnitDefaultJobs          int              `json:"unitDefaultJobs"`
	ComputedLaneBudget       int              `json:"computedLaneBudget"`
	Runner                   unitTimingRunner `json:"runner"`
	EnvironmentInvalidations []string         `json:"environmentInvalidations"`
}

// unitTimingSummary is the versioned machine-readable shape emitted by
// unitlane. PackageElapsedSecondsSum is expected to exceed WallSeconds when
// independent packages overlap under go test's package scheduler.
type unitTimingSummary struct {
	Version                  int                 `json:"version"`
	Complete                 bool                `json:"complete"`
	Run                      unitTimingRun       `json:"run"`
	WallSeconds              float64             `json:"wallSeconds"`
	PackageElapsedSecondsSum float64             `json:"packageElapsedSecondsSum"`
	PackageCount             int                 `json:"packageCount"`
	ExpectedPackageCount     int                 `json:"expectedPackageCount"`
	TestCount                int                 `json:"testCount"`
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
	tests          map[string]map[string]struct{}
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

func (accumulator *unitTimingAccumulator) summaryWithRun(wallSeconds float64, run unitTimingRun) unitTimingSummary {
	packages := make([]unitPackageTiming, 0, len(accumulator.packages))
	packageSum := 0.0
	testCount := 0
	for _, expectedPackage := range accumulator.expected {
		packageTiming, exists := accumulator.packages[expectedPackage]
		if !exists {
			accumulator.complete = false
			continue
		}
		packages = append(packages, packageTiming)
		packageSum += packageTiming.Seconds
		testCount += len(packageTiming.Tests)
	}
	if len(accumulator.packages) != len(packages) {
		accumulator.complete = false
	}
	rankUnitPackageTimings(packages)
	return unitTimingSummary{
		Version:                  unitTimingSummaryVersion,
		Complete:                 accumulator.complete,
		Run:                      run,
		WallSeconds:              roundUnitTimingSeconds(wallSeconds),
		PackageElapsedSecondsSum: roundUnitTimingSeconds(packageSum),
		PackageCount:             len(packages),
		ExpectedPackageCount:     len(accumulator.expected),
		TestCount:                testCount,
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
		tests:          make(map[string]map[string]struct{}, len(expectedPackages)),
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
	if event.Test != "" {
		if isUnitTimingTerminal(event.Action) {
			state.recordTestTerminal(event)
		}
		return
	}
	if isUnitTimingTerminal(event.Action) {
		state.recordTerminal(event)
	}
}

func (state *unitTimingEventState) recordTestTerminal(event goTestUnitTimingEvent) {
	if !state.validTestEvent(event) || math.IsNaN(event.Elapsed) || math.IsInf(event.Elapsed, 0) || event.Elapsed < 0 {
		state.complete = false
		return
	}
	tests := state.tests[event.Package]
	if tests == nil {
		tests = make(map[string]struct{})
		state.tests[event.Package] = tests
	}
	if _, exists := tests[event.Test]; exists {
		return
	}
	tests[event.Test] = struct{}{}
}

func (state *unitTimingEventState) validTestEvent(event goTestUnitTimingEvent) bool {
	if strings.TrimSpace(event.Package) == "" || strings.TrimSpace(event.Test) == "" {
		state.complete = false
		return false
	}
	if _, exists := state.expected[event.Package]; !exists {
		state.complete = false
		return false
	}
	return true
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
		packageTiming.Tests = sortedUnitTestNames(state.tests[packageName])
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

func sortUnitPackageTimings(packages []unitPackageTiming) {
	slices.SortFunc(packages, func(left, right unitPackageTiming) int {
		return strings.Compare(left.Package, right.Package)
	})
}

func sortedUnitTestNames(tests map[string]struct{}) []string {
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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
	ordered := summary
	ordered.Packages = append([]unitPackageTiming(nil), summary.Packages...)
	sortUnitPackageTimings(ordered.Packages)
	for index := range ordered.Packages {
		ordered.Packages[index].Tests = append([]string(nil), ordered.Packages[index].Tests...)
		slices.Sort(ordered.Packages[index].Tests)
	}
	data, err := json.MarshalIndent(ordered, "", "  ")
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
	return atomicWriteUnitTimingFile(path, data)
}

func atomicWriteUnitTimingFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".unit-timing-*.tmp")
	if err != nil {
		return fmt.Errorf("create unit timing temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set unit timing temporary file mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write unit timing temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close unit timing temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("rename unit timing temporary file: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace unit timing output: %w", removeErr)
		}
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("rename unit timing temporary file: %w", retryErr)
		}
	}
	return nil
}

func unitTimingRunIdentity(cfg config) unitTimingRun {
	commit, commitErr := unitTimingCommit()
	invalidations := unitTimingInvalidations()
	if invalidations == nil {
		invalidations = []string{}
	}
	if commitErr != nil {
		invalidations = append(invalidations, "commit unavailable: "+commitErr.Error())
	}
	slices.Sort(invalidations)
	return unitTimingRun{
		Commit:                   commit,
		GoVersion:                unitTimingGoVersion(),
		Command:                  unitTimingCommand(cfg),
		UnitDefaultJobs:          cfg.jobs,
		ComputedLaneBudget:       unitTimingComputedBudget(cfg),
		Runner:                   unitTimingRunnerIdentity(),
		EnvironmentInvalidations: slices.Compact(invalidations),
	}
}

func unitTimingCommit() (string, error) {
	if commit := strings.TrimSpace(os.Getenv("UNIT_TIMING_COMMIT")); commit != "" {
		return commit, nil
	}
	if commit := strings.TrimSpace(os.Getenv("GITHUB_SHA")); commit != "" {
		return commit, nil
	}
	output, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func unitTimingGoVersion() string {
	version := runtime.Version()
	if index := strings.IndexAny(version, " \t"); index >= 0 {
		version = version[:index]
	}
	return version
}

func unitTimingCommand(cfg config) string {
	if command := strings.TrimSpace(cfg.timingCommand); command != "" {
		return command
	}
	if command := strings.TrimSpace(os.Getenv("UNIT_TIMING_COMMAND")); command != "" {
		return command
	}
	return strings.Join(os.Args, " ")
}

func unitTimingComputedBudget(cfg config) int {
	if cfg.computedLaneBudget > 0 {
		return cfg.computedLaneBudget
	}
	if raw, ok := os.LookupEnv("GO_LANE_BUDGET"); ok {
		if budget, valid := positiveDecimal(raw); valid {
			return budget
		}
	}
	expectedLanes := os.Getenv(expectedConcurrentLanesEnv)
	return boundedUnitLaneJobs(runtime.NumCPU(), expectedLanes)
}

func unitTimingRunnerIdentity() unitTimingRunner {
	provider := strings.TrimSpace(os.Getenv("UNIT_RUNNER_PROVIDER"))
	if provider == "" {
		provider = "local"
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			provider = "github-actions"
		}
	}
	image := strings.TrimSpace(os.Getenv("UNIT_RUNNER_IMAGE"))
	if image == "" {
		image = "local"
	}
	imageVersion := strings.TrimSpace(os.Getenv("UNIT_RUNNER_IMAGE_VERSION"))
	if imageVersion == "" {
		imageVersion = strings.TrimSpace(os.Getenv("ImageVersion"))
	}
	if imageVersion == "" {
		imageVersion = "unknown"
	}
	cpuModel := strings.TrimSpace(os.Getenv("UNIT_RUNNER_CPU_MODEL"))
	if cpuModel == "" {
		cpuModel = hostCPUModel()
	}
	return unitTimingRunner{
		Provider:     provider,
		Image:        image,
		ImageVersion: imageVersion,
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPUModel:     cpuModel,
	}
}

func hostCPUModel() string {
	if runtime.GOOS != "linux" {
		return "unknown"
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(key), "model name") && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "unknown"
}

func unitTimingInvalidations() []string {
	raw := strings.TrimSpace(os.Getenv("UNIT_TIMING_INVALIDATIONS"))
	if raw == "" {
		return nil
	}
	var invalidations []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			invalidations = append(invalidations, value)
		}
	}
	return invalidations
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
