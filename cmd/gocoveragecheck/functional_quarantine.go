package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	functionalQuarantineVersion           = 1
	functionalSuiteName                   = "functional"
	functionalBucketEnvironment           = "ENVIRONMENT-DEPENDENT"
	functionalBucketFailure               = "GENUINELY FAILING"
	functionalMeasurementIsolated         = "isolated"
	functionalMeasurementPackageContext   = "package-context"
	functionalMeasurementRepeatedIsolated = "repeated-isolated"
	functionalQuarantineMinAttempts       = 2
	functionalQuarantineMaxAttempts       = 15
)

var functionalTestNamePattern = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

type functionalQuarantine struct {
	Version int                         `json:"version"`
	Suite   string                      `json:"suite"`
	Entries []functionalQuarantineEntry `json:"entries"`
}

type functionalQuarantineEntry struct {
	Package     string `json:"package"`
	Test        string `json:"test,omitempty"`
	Bucket      string `json:"bucket"`
	Reason      string `json:"reason"`
	FollowUp    string `json:"followUp,omitempty"`
	Measurement string `json:"measurement,omitempty"`
	Attempts    int    `json:"attempts,omitempty"`
}

type functionalTestInventory struct {
	Packages []string
	Tests    map[string][]string
}

type functionalCoverageSelection struct {
	Inventory                functionalTestInventory
	SelectedTests            map[string][]string
	Groups                   []coverageRunGroup
	QuarantinedPackageCount  int
	QuarantinedTestSelectors int
	PackageExcludedTestCount int
	TestExcludedPackageCount int
	SelectedPackageCount     int
	SelectedTestCount        int
}

type coverageRunGroup struct {
	Packages   []string
	RunPattern string
}

type goTestListEvent struct {
	Action  string `json:"Action"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func resolveFunctionalCoverageSelection(path string, listPatterns, packages []string, timeout time.Duration, short bool, jobs int, repoRoot string) (functionalCoverageSelection, functionalQuarantine, error) {
	return resolveFunctionalCoverageSelectionWithMetadata(path, listPatterns, packages, timeout, short, jobs, repoRoot, nil)
}

func resolveFunctionalCoverageSelectionWithMetadata(path string, listPatterns, packages []string, timeout time.Duration, short bool, jobs int, repoRoot string, listedPackages []functionalGoListPackage) (functionalCoverageSelection, functionalQuarantine, error) {
	manifest, err := readFunctionalQuarantineFile(path)
	if err != nil {
		return functionalCoverageSelection{}, functionalQuarantine{}, err
	}
	var inventory functionalTestInventory
	if listedPackages == nil {
		inventory, err = discoverFunctionalTestInventoryWithPatternsAndJobs(listPatterns, packages, jobs, repoRoot)
	} else {
		inventory, err = discoverFunctionalTestInventoryFromListedPackagesWithJobs(packages, listedPackages, jobs)
	}
	if err != nil {
		return functionalCoverageSelection{}, functionalQuarantine{}, err
	}
	if err := validateFunctionalQuarantine(manifest, inventory); err != nil {
		return functionalCoverageSelection{}, functionalQuarantine{}, err
	}
	if err := verifyFunctionalTestQuarantineSelectors(manifest, timeout, short, jobs, repoRoot); err != nil {
		return functionalCoverageSelection{}, functionalQuarantine{}, err
	}
	selection, err := buildFunctionalCoverageSelection(manifest, inventory)
	if err != nil {
		return functionalCoverageSelection{}, functionalQuarantine{}, err
	}
	return selection, manifest, nil
}

func prepareFunctionalCoverageRun(cfg config, packages []string, targetOS string, logicalCPUs int, repoRoot string) (functionalCoverageSelection, []string, error) {
	return prepareFunctionalCoverageRunAfterStart(
		cfg,
		packages,
		targetOS,
		logicalCPUs,
		repoRoot,
		nil,
		startFunctionalDiscovery(strconv.Itoa(len(sortedUniqueStrings(packages)))),
	)
}

func prepareFunctionalCoverageRunAfterStart(cfg config, packages []string, targetOS string, logicalCPUs int, repoRoot string, listedPackages []functionalGoListPackage, discoveryStarted time.Time) (functionalCoverageSelection, []string, error) {
	quarantinePath := cfg.functionalQuarantine
	if !filepath.IsAbs(quarantinePath) {
		quarantinePath = filepath.Join(repoRoot, quarantinePath)
	}
	var selection functionalCoverageSelection
	var manifest functionalQuarantine
	var err error
	if listedPackages == nil {
		selection, manifest, err = resolveFunctionalCoverageSelection(
			quarantinePath,
			packages,
			packages,
			cfg.timeout,
			cfg.short,
			cfg.testJobs(targetOS, logicalCPUs),
			repoRoot,
		)
	} else {
		selection, manifest, err = resolveFunctionalCoverageSelectionWithMetadata(
			quarantinePath,
			packages,
			packages,
			cfg.timeout,
			cfg.short,
			cfg.testJobs(targetOS, logicalCPUs),
			repoRoot,
			listedPackages,
		)
	}
	if err != nil {
		writeFunctionalDiscoveryEnd(discoveryStarted, "failed", functionalTestInventory{})
		return functionalCoverageSelection{}, nil, err
	}
	writeFunctionalDiscoveryEnd(discoveryStarted, "complete", selection.Inventory)
	fmt.Fprintf(
		stdoutWriter,
		"Functional gate: discovered-packages=%d discovered-tests=%d quarantined-packages=%d quarantined-test-selectors=%d package-excluded-tests=%d test-excluded-packages=%d selected-packages=%d selected-tests=%d selection=subtractive quarantine=%s\n",
		len(selection.Inventory.Packages),
		functionalTestCount(selection.Inventory),
		selection.QuarantinedPackageCount,
		selection.QuarantinedTestSelectors,
		selection.PackageExcludedTestCount,
		selection.TestExcludedPackageCount,
		selection.SelectedPackageCount,
		selection.SelectedTestCount,
		filepath.ToSlash(cfg.functionalQuarantine),
	)
	if err := runFunctionalQuarantineRatchet(manifest, cfg.timeout, cfg.short, repoRoot); err != nil {
		return functionalCoverageSelection{}, nil, err
	}
	return selection, selectedFunctionalPackages(selection), nil
}

func resolveCoverageTestPackages(cfg config, repoRoot string) ([]string, []functionalGoListPackage, time.Time, error) {
	if strings.TrimSpace(cfg.functionalQuarantine) == "" {
		packages, err := resolveTestPackages(cfg)
		return packages, nil, time.Time{}, err
	}

	discoveryStarted := startFunctionalDiscovery(functionalDiscoveryRequestLabel(cfg))
	packages, listedPackages, err := resolveFunctionalTestPackagesWithMetadata(cfg, repoRoot)
	if err != nil {
		writeFunctionalDiscoveryEnd(discoveryStarted, "failed", functionalTestInventory{})
		return nil, nil, time.Time{}, err
	}
	return packages, listedPackages, discoveryStarted, nil
}

func prepareCoverageTestPackages(cfg config, packages []string, targetOS string, logicalCPUs int, repoRoot string, listedPackages []functionalGoListPackage, discoveryStarted time.Time) ([]string, *functionalCoverageSelection, error) {
	if strings.TrimSpace(cfg.functionalQuarantine) == "" {
		return packages, nil, nil
	}
	selection, selectedPackages, err := prepareFunctionalCoverageRunAfterStart(cfg, packages, targetOS, logicalCPUs, repoRoot, listedPackages, discoveryStarted)
	if err != nil {
		return nil, nil, err
	}
	return selectedPackages, &selection, nil
}

func startFunctionalDiscovery(requestedPackages string) time.Time {
	if strings.TrimSpace(requestedPackages) == "" {
		requestedPackages = "unknown"
	}
	started := time.Now()
	fmt.Fprintf(stdoutWriter, "Functional discovery: begin requested-packages=%s\n", requestedPackages)
	return started
}

func functionalDiscoveryRequestLabel(cfg config) string {
	if strings.TrimSpace(cfg.packages) == "" {
		return "current-tree"
	}
	return strconv.Itoa(len(splitList(cfg.packages, " ", true)))
}

func writeFunctionalDiscoveryEnd(started time.Time, status string, inventory functionalTestInventory) {
	if status == "" {
		status = "failed"
	}
	elapsed := time.Since(started).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	fmt.Fprintf(
		stdoutWriter,
		"Functional discovery: end status=%s elapsed=%.3fs discovered-packages=%d discovered-tests=%d\n",
		status,
		elapsed,
		len(inventory.Packages),
		functionalTestCount(inventory),
	)
}

const (
	functionalQuarantineOutcomePass = timingOutcomePass
	functionalQuarantineOutcomeFail = timingOutcomeFail
	functionalQuarantineOutcomeSkip = timingOutcomeSkip
)

type functionalQuarantineOutcomeResult struct {
	Entry                   functionalQuarantineEntry
	Observed                string
	Detail                  string
	TestFailureObserved     bool
	PackageTerminalObserved bool
	OutcomeCounts           functionalQuarantineOutcomeCounts
}

type functionalQuarantineOutcomeCounts struct {
	Attempts int
	Passes   int
	Failures int
	Skips    int
}

func (counts *functionalQuarantineOutcomeCounts) add(outcome string) {
	counts.Attempts++
	switch outcome {
	case functionalQuarantineOutcomePass:
		counts.Passes++
	case functionalQuarantineOutcomeFail:
		counts.Failures++
	case functionalQuarantineOutcomeSkip:
		counts.Skips++
	}
}

func (counts functionalQuarantineOutcomeCounts) observed() (string, error) {
	// A pass is recovery only when every attempt passes. A failure takes
	// precedence over skips so an expected failure keeps an intermittent
	// selector quarantined instead of flapping the gate.
	switch {
	case counts.Attempts == 0:
		return "", errors.New("measurement produced no attempts")
	case counts.Passes == counts.Attempts:
		return functionalQuarantineOutcomePass, nil
	case counts.Failures > 0:
		return functionalQuarantineOutcomeFail, nil
	case counts.Skips > 0:
		return functionalQuarantineOutcomeSkip, nil
	default:
		return "", errors.New("measurement produced an unknown outcome")
	}
}

func (result functionalQuarantineOutcomeResult) allAttemptsPassed() bool {
	return result.OutcomeCounts.Attempts > 0 &&
		result.OutcomeCounts.Passes == result.OutcomeCounts.Attempts
}

func runFunctionalQuarantineRatchet(manifest functionalQuarantine, timeout time.Duration, short bool, repoRoot string) error {
	if len(manifest.Entries) == 0 {
		return nil
	}

	results := make([]functionalQuarantineOutcomeResult, 0, len(manifest.Entries))
	var failures []error
	for _, entry := range manifest.Entries {
		measurement, measurementErr := functionalQuarantineMeasurement(entry)
		if measurementErr != nil {
			failures = append(failures, fmt.Errorf("functional quarantine ratchet: selector %q has invalid measurement metadata (fail-closed): %w", functionalSelectorDisplay(entry), measurementErr))
			fmt.Fprintf(stdoutWriter, "Functional quarantine selector: selector=%q bucket=%s observed=validation-error status=fail-closed measurement=%q detail=%q\n", functionalSelectorDisplay(entry), entry.Bucket, entry.Measurement, compactFunctionalQuarantineDetail(measurementErr.Error()))
			continue
		}

		result, err := runFunctionalQuarantineSelectorWithMeasurementSpec(entry, measurement, timeout, short, repoRoot)
		if err != nil {
			failures = append(failures, fmt.Errorf("functional quarantine ratchet: selector %q execution error (fail-closed): %w", functionalSelectorDisplay(entry), err))
			fmt.Fprintf(stdoutWriter, "Functional quarantine selector: selector=%q bucket=%s observed=execution-error status=fail-closed measurement=%s detail=%q\n", functionalSelectorDisplay(entry), entry.Bucket, measurement.method, compactFunctionalQuarantineDetail(err.Error()))
			continue
		}

		results = append(results, result)
		expected, expectedErr := expectedFunctionalQuarantineOutcome(entry.Bucket)
		if expectedErr != nil {
			failures = append(failures, fmt.Errorf("functional quarantine ratchet: selector %q has no expected outcome for bucket %q: %w", functionalSelectorDisplay(entry), entry.Bucket, expectedErr))
			fmt.Fprintf(stdoutWriter, "Functional quarantine selector: selector=%q bucket=%s observed=%s status=fail-closed measurement=%s detail=%q\n", functionalSelectorDisplay(entry), entry.Bucket, result.Observed, measurement.method, compactFunctionalQuarantineDetail(expectedErr.Error()))
			continue
		}

		status := "expected"
		switch {
		case result.allAttemptsPassed():
			status = "unexpected-pass"
		case result.Observed == expected:
		case unmeasurableFunctionalQuarantineOutcome(short, expected, result.Observed):
			// Under -short the selector never executes, so "did it fail?" is
			// unmeasurable rather than answered. Treating that as a violation
			// makes a GENUINELY FAILING entry impossible to validate on the PR
			// tier, which runs -short while pushes do not. Report it and move
			// on; unexpected-pass above still catches stale entries.
			status = "unmeasurable-under-short"
		default:
			status = "unexpected-outcome"
		}
		if status == "unexpected-pass" {
			failures = append(failures, fmt.Errorf("functional quarantine ratchet: selector %q passed unexpectedly (bucket=%s); remove or narrow this quarantine entry", functionalSelectorDisplay(entry), entry.Bucket))
		} else if status == "unexpected-outcome" {
			failures = append(failures, fmt.Errorf("functional quarantine ratchet: selector %q observed %s, expected %s for bucket=%s; verify the quarantine bucket and precondition", functionalSelectorDisplay(entry), result.Observed, expected, entry.Bucket))
		}
		fmt.Fprintf(stdoutWriter, "Functional quarantine selector: selector=%q bucket=%s expected=%s observed=%s status=%s %s detail=%q\n", functionalSelectorDisplay(entry), entry.Bucket, expected, result.Observed, status, formatFunctionalQuarantineMeasurement(measurement, result.OutcomeCounts), compactFunctionalQuarantineDetail(result.Detail))
	}

	passCount, failCount, skipCount := 0, 0, 0
	for _, result := range results {
		switch result.Observed {
		case functionalQuarantineOutcomePass:
			passCount++
		case functionalQuarantineOutcomeFail:
			failCount++
		case functionalQuarantineOutcomeSkip:
			skipCount++
		}
	}
	fmt.Fprintf(stdoutWriter, "Functional quarantine ratchet: selectors=%d observed-pass=%d observed-fail=%d observed-skip=%d execution-errors=%d\n", len(manifest.Entries), passCount, failCount, skipCount, len(manifest.Entries)-len(results))
	return errors.Join(failures...)
}

func runFunctionalQuarantineSelector(entry functionalQuarantineEntry, timeout time.Duration, short bool, repoRoot string) (functionalQuarantineOutcomeResult, error) {
	measurement, err := functionalQuarantineMeasurement(entry)
	if err != nil {
		return functionalQuarantineOutcomeResult{}, err
	}
	return runFunctionalQuarantineSelectorWithMeasurementSpec(entry, measurement, timeout, short, repoRoot)
}

type functionalQuarantineMeasurementSpec struct {
	method   string
	attempts int
}

func runFunctionalQuarantineSelectorWithMeasurementSpec(entry functionalQuarantineEntry, measurement functionalQuarantineMeasurementSpec, timeout time.Duration, short bool, repoRoot string) (functionalQuarantineOutcomeResult, error) {
	if measurement.method != functionalMeasurementRepeatedIsolated {
		return runFunctionalQuarantineSelectorWithMeasurement(entry, measurement.method, timeout, short, repoRoot)
	}

	aggregate := functionalQuarantineOutcomeResult{Entry: entry}
	for attempt := 1; attempt <= measurement.attempts; attempt++ {
		result, err := runFunctionalQuarantineSelectorWithMeasurement(entry, functionalMeasurementIsolated, timeout, short, repoRoot)
		if err != nil {
			return functionalQuarantineOutcomeResult{}, fmt.Errorf("repeated-isolated measurement attempt %d/%d failed: %w", attempt, measurement.attempts, err)
		}
		aggregate.OutcomeCounts.merge(result.OutcomeCounts)
		if aggregate.Detail == "" {
			aggregate.Detail = result.Detail
		}
		aggregate.TestFailureObserved = aggregate.TestFailureObserved || result.TestFailureObserved
		aggregate.PackageTerminalObserved = aggregate.PackageTerminalObserved || result.PackageTerminalObserved
	}
	observed, err := aggregate.OutcomeCounts.observed()
	if err != nil {
		return functionalQuarantineOutcomeResult{}, fmt.Errorf("repeated-isolated measurement: %w", err)
	}
	aggregate.Observed = observed
	return aggregate, nil
}

func (counts *functionalQuarantineOutcomeCounts) merge(other functionalQuarantineOutcomeCounts) {
	counts.Attempts += other.Attempts
	counts.Passes += other.Passes
	counts.Failures += other.Failures
	counts.Skips += other.Skips
}

func runFunctionalQuarantineSelectorWithMeasurement(entry functionalQuarantineEntry, measurement string, timeout time.Duration, short bool, repoRoot string) (functionalQuarantineOutcomeResult, error) {
	args := []string{"test", "-json", "-count=1"}
	if short {
		args = append(args, "-short")
	}
	args = append(args, fmt.Sprintf("-timeout=%s", timeout))
	if entry.Test != "" && measurement != functionalMeasurementPackageContext {
		args = append(args, "-run="+exactFunctionalTestRunPattern([]string{entry.Test}))
	}
	args = append(args, entry.Package)

	stdout, stderr, commandErr := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  repoRoot,
	})
	result, parseErr := parseFunctionalQuarantineOutcome(stdout, entry)
	if parseErr != nil {
		if commandErr != nil {
			return functionalQuarantineOutcomeResult{}, errors.Join(parseErr, fmt.Errorf("%w: %s", commandErr, compactFunctionalQuarantineDetail(mergeGoTestFailureDetail(stderr, stdout))))
		}
		return functionalQuarantineOutcomeResult{}, parseErr
	}
	if measurement == functionalMeasurementPackageContext && !result.PackageTerminalObserved {
		return functionalQuarantineOutcomeResult{}, errors.New("package-context measurement produced no package terminal outcome; execution was incomplete")
	}
	if commandErr != nil && (result.Observed != functionalQuarantineOutcomeFail || !result.TestFailureObserved) {
		if measurement == functionalMeasurementPackageContext && result.PackageTerminalObserved {
			return result, nil
		}
		return functionalQuarantineOutcomeResult{}, fmt.Errorf("go test returned an execution error after observing %s without a failing test event: %s", result.Observed, compactFunctionalQuarantineDetail(mergeGoTestFailureDetail(stderr, stdout)))
	}
	return result, nil
}

func parseFunctionalQuarantineOutcome(jsonOutput string, entry functionalQuarantineEntry) (functionalQuarantineOutcomeResult, error) {
	var result functionalQuarantineOutcomeResult
	result.Entry = entry
	var terminal *goTestTimingEvent
	var packageTerminal *goTestTimingEvent
	for _, line := range strings.Split(jsonOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event goTestTimingEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return functionalQuarantineOutcomeResult{}, fmt.Errorf("decode go test JSON output: %w", err)
		}
		if event.Package != entry.Package {
			return functionalQuarantineOutcomeResult{}, fmt.Errorf("go test reported unexpected package %q for selector %q", event.Package, functionalSelectorDisplay(entry))
		}
		if event.Test != "" && event.Action == functionalQuarantineOutcomeFail {
			result.TestFailureObserved = true
		}
		if event.Test == "" {
			switch event.Action {
			case functionalQuarantineOutcomePass, functionalQuarantineOutcomeFail, functionalQuarantineOutcomeSkip:
				if packageTerminal != nil {
					return functionalQuarantineOutcomeResult{}, fmt.Errorf("package %q reported duplicate terminal outcomes", entry.Package)
				}
				copy := event
				packageTerminal = &copy
			}
		}
		if entry.Test != "" && event.Test != entry.Test {
			continue
		}
		if entry.Test == "" && event.Test != "" {
			continue
		}
		switch event.Action {
		case functionalQuarantineOutcomePass, functionalQuarantineOutcomeFail, functionalQuarantineOutcomeSkip:
			if terminal != nil {
				return functionalQuarantineOutcomeResult{}, fmt.Errorf("selector %q reported duplicate terminal outcomes", functionalSelectorDisplay(entry))
			}
			copy := event
			terminal = &copy
			result.Detail = firstTimingFailureReason(event.Output)
		}
	}
	if terminal == nil {
		return functionalQuarantineOutcomeResult{}, fmt.Errorf("selector %q produced no terminal outcome; it may be stale or execution was incomplete", functionalSelectorDisplay(entry))
	}
	result.Observed = terminal.Action
	result.PackageTerminalObserved = packageTerminal != nil
	result.OutcomeCounts.add(result.Observed)
	return result, nil
}

func functionalQuarantineMeasurement(entry functionalQuarantineEntry) (functionalQuarantineMeasurementSpec, error) {
	switch entry.Measurement {
	case "":
		if entry.Attempts != 0 {
			return functionalQuarantineMeasurementSpec{}, fmt.Errorf("attempts=%d requires %q measurement", entry.Attempts, functionalMeasurementRepeatedIsolated)
		}
		return functionalQuarantineMeasurementSpec{method: functionalMeasurementIsolated, attempts: 1}, nil
	case functionalMeasurementIsolated:
		if entry.Attempts != 0 {
			return functionalQuarantineMeasurementSpec{}, fmt.Errorf("attempts=%d requires %q measurement", entry.Attempts, functionalMeasurementRepeatedIsolated)
		}
		return functionalQuarantineMeasurementSpec{method: functionalMeasurementIsolated, attempts: 1}, nil
	case functionalMeasurementPackageContext:
		if entry.Test == "" {
			return functionalQuarantineMeasurementSpec{}, errors.New("package-context measurement requires a test selector")
		}
		if entry.Attempts != 0 {
			return functionalQuarantineMeasurementSpec{}, fmt.Errorf("attempts=%d requires %q measurement", entry.Attempts, functionalMeasurementRepeatedIsolated)
		}
		return functionalQuarantineMeasurementSpec{method: functionalMeasurementPackageContext, attempts: 1}, nil
	case functionalMeasurementRepeatedIsolated:
		if entry.Test == "" {
			return functionalQuarantineMeasurementSpec{}, errors.New("repeated-isolated measurement requires a test selector")
		}
		if entry.Attempts < functionalQuarantineMinAttempts || entry.Attempts > functionalQuarantineMaxAttempts {
			return functionalQuarantineMeasurementSpec{}, fmt.Errorf("repeated-isolated measurement attempts=%d is invalid; expected a bounded count from %d through %d", entry.Attempts, functionalQuarantineMinAttempts, functionalQuarantineMaxAttempts)
		}
		return functionalQuarantineMeasurementSpec{method: functionalMeasurementRepeatedIsolated, attempts: entry.Attempts}, nil
	default:
		return functionalQuarantineMeasurementSpec{}, fmt.Errorf("unsupported measurement %q; expected %q, %q, or %q", entry.Measurement, functionalMeasurementIsolated, functionalMeasurementPackageContext, functionalMeasurementRepeatedIsolated)
	}
}

func formatFunctionalQuarantineMeasurement(measurement functionalQuarantineMeasurementSpec, counts functionalQuarantineOutcomeCounts) string {
	if measurement.attempts == 1 {
		return fmt.Sprintf("measurement=%s", measurement.method)
	}
	return fmt.Sprintf("measurement=%s attempts=%d pass=%d fail=%d skip=%d", measurement.method, counts.Attempts, counts.Passes, counts.Failures, counts.Skips)
}

// unmeasurableFunctionalQuarantineOutcome reports whether a quarantined
// selector's outcome could not be measured because the run was short-mode.
//
// The functional tier differs between triggers: a pull request runs -short and
// a push to the default branch does not. A test that self-skips under
// testing.Short() is therefore observed as a skip on the PR tier no matter
// whether it fails, so asserting expected=fail there asks an unanswerable
// question. Only a GENUINELY FAILING entry is affected; an ENVIRONMENT-DEPENDENT
// entry already expects a skip.
func unmeasurableFunctionalQuarantineOutcome(short bool, expected, observed string) bool {
	return short &&
		expected == functionalQuarantineOutcomeFail &&
		observed == functionalQuarantineOutcomeSkip
}

func expectedFunctionalQuarantineOutcome(bucket string) (string, error) {
	switch bucket {
	case functionalBucketEnvironment:
		return functionalQuarantineOutcomeSkip, nil
	case functionalBucketFailure:
		return functionalQuarantineOutcomeFail, nil
	default:
		return "", fmt.Errorf("unsupported quarantine bucket")
	}
}

func compactFunctionalQuarantineDetail(detail string) string {
	detail = strings.TrimSpace(strings.ReplaceAll(detail, "\n", " "))
	if len(detail) > maxTimingFailureReasonLength {
		return detail[:maxTimingFailureReasonLength] + "..."
	}
	return detail
}

func readFunctionalQuarantineFile(path string) (functionalQuarantine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return functionalQuarantine{}, fmt.Errorf("read functional quarantine %q: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest functionalQuarantine
	if err := decoder.Decode(&manifest); err != nil {
		return functionalQuarantine{}, fmt.Errorf("parse functional quarantine %q: %w", path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return functionalQuarantine{}, fmt.Errorf("parse functional quarantine %q: %w", path, err)
	}
	if manifest.Entries == nil {
		return functionalQuarantine{}, fmt.Errorf("parse functional quarantine %q: entries must be an array", path)
	}
	return manifest, nil
}

func parseFunctionalTestList(output string, inventory functionalTestInventory, seenPackages map[string]struct{}) error {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event goTestListEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("discover functional tests: decode go test list event: %w", err)
		}
		if event.Package == "" {
			continue
		}
		if _, known := inventory.Tests[event.Package]; !known {
			return fmt.Errorf("discover functional tests: go test reported unexpected package %q", event.Package)
		}
		if event.Test == "" && isFunctionalTestListTerminal(event.Action) {
			seenPackages[event.Package] = struct{}{}
		}
		if event.Test != "" {
			if err := addFunctionalTestName(inventory.Tests, event.Package, event.Test); err != nil {
				return err
			}
		}
		for _, outputLine := range strings.Split(event.Output, "\n") {
			name := strings.TrimSpace(outputLine)
			if name == "" {
				continue
			}
			if functionalTestNamePattern.MatchString(name) {
				if err := addFunctionalTestName(inventory.Tests, event.Package, name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func verifyFunctionalTestQuarantineSelectors(manifest functionalQuarantine, timeout time.Duration, short bool, jobs int, repoRoot string) error {
	selectorPackages := make([]string, 0)
	for _, entry := range manifest.Entries {
		if entry.Test != "" {
			selectorPackages = append(selectorPackages, entry.Package)
		}
	}
	selectorPackages = sortedUniqueStrings(selectorPackages)
	if len(selectorPackages) == 0 {
		return nil
	}

	runtimeInventory, err := discoverFunctionalTestInventoryByRuntimeList(selectorPackages, timeout, short, jobs, repoRoot)
	if err != nil {
		return err
	}
	for _, entry := range manifest.Entries {
		if entry.Test == "" {
			continue
		}
		if !slices.Contains(runtimeInventory.Tests[entry.Package], entry.Test) {
			return fmt.Errorf("validate functional quarantine: selector %q is not discoverable in package %q", entry.Test, entry.Package)
		}
	}
	return nil
}

func discoverFunctionalTestInventoryByRuntimeList(packages []string, timeout time.Duration, short bool, jobs int, repoRoot string) (functionalTestInventory, error) {
	packages = sortedUniqueStrings(packages)
	if len(packages) == 0 {
		return functionalTestInventory{}, errors.New("discover functional tests: no packages were selected for runtime verification")
	}

	// This retained runtime path verifies test registration for quarantine
	// selectors only; the coverage and lint lanes own vet execution. Skipping
	// the redundant vet pass keeps selector verification bounded without
	// weakening the runtime listing or its fail-closed parsing checks.
	args := []string{"test", "-vet=off", "-list=^Test", "-json", fmt.Sprintf("-p=%d", maxFunctionalDiscoveryJobs(jobs)), "-count=1"}
	if short {
		args = append(args, "-short")
	}
	args = append(args, fmt.Sprintf("-timeout=%s", timeout))
	args = append(args, packages...)
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  repoRoot,
	})
	if err != nil {
		detail := mergeGoTestFailureDetail(stderr, stdout)
		if detail != "" {
			return functionalTestInventory{}, fmt.Errorf("discover functional tests: runtime go test list: %w\n%s", err, detail)
		}
		return functionalTestInventory{}, fmt.Errorf("discover functional tests: runtime go test list: %w", err)
	}

	inventory := functionalTestInventory{
		Packages: packages,
		Tests:    make(map[string][]string, len(packages)),
	}
	for _, packagePath := range packages {
		inventory.Tests[packagePath] = nil
	}
	seenPackages := make(map[string]struct{}, len(packages))
	if err := parseFunctionalTestList(stdout, inventory, seenPackages); err != nil {
		return functionalTestInventory{}, err
	}
	for _, packagePath := range packages {
		if _, ok := seenPackages[packagePath]; !ok {
			return functionalTestInventory{}, fmt.Errorf("discover functional tests: package %q did not report a terminal runtime list event", packagePath)
		}
		inventory.Tests[packagePath] = sortedUniqueStrings(inventory.Tests[packagePath])
	}
	return inventory, nil
}

func maxFunctionalDiscoveryJobs(jobs int) int {
	if jobs < 1 {
		return 1
	}
	return jobs
}

func isFunctionalTestListTerminal(action string) bool {
	switch action {
	case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		return true
	default:
		return false
	}
}

func addFunctionalTestName(tests map[string][]string, packagePath, name string) error {
	if !functionalTestNamePattern.MatchString(name) {
		return fmt.Errorf("discover functional tests: package %q reported invalid top-level test name %q", packagePath, name)
	}
	tests[packagePath] = append(tests[packagePath], name)
	return nil
}

func validateFunctionalQuarantine(manifest functionalQuarantine, inventory functionalTestInventory) error {
	if manifest.Version != functionalQuarantineVersion {
		return fmt.Errorf("validate functional quarantine: version %d is unsupported; expected %d", manifest.Version, functionalQuarantineVersion)
	}
	if manifest.Suite != functionalSuiteName {
		return fmt.Errorf("validate functional quarantine: suite %q is unsupported; expected %q", manifest.Suite, functionalSuiteName)
	}
	knownPackages := make(map[string]struct{}, len(inventory.Packages))
	for _, packagePath := range inventory.Packages {
		knownPackages[packagePath] = struct{}{}
	}
	seen := make(map[string]struct{}, len(manifest.Entries))
	packageSelectors := make(map[string]struct{})
	previous := ""
	for index, entry := range manifest.Entries {
		if err := validateFunctionalQuarantineEntry(entry, index, knownPackages, inventory.Tests); err != nil {
			return err
		}
		key := functionalSelectorKey(entry.Package, entry.Test)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("validate functional quarantine: duplicate selector %q", functionalSelectorDisplay(entry))
		}
		if previous != "" && key < previous {
			return fmt.Errorf("validate functional quarantine: entries must be sorted by package and test; %q appears after %q", key, previous)
		}
		if entry.Test == "" {
			packageSelectors[entry.Package] = struct{}{}
		} else if _, packageSelected := packageSelectors[entry.Package]; packageSelected {
			return fmt.Errorf("validate functional quarantine: package selector %q overlaps test selector %q; remove the test entry or quarantine the package only", entry.Package, functionalSelectorDisplay(entry))
		}
		seen[key] = struct{}{}
		previous = key
	}
	return nil
}

func validateFunctionalQuarantineEntry(entry functionalQuarantineEntry, index int, knownPackages map[string]struct{}, tests map[string][]string) error {
	if strings.TrimSpace(entry.Package) == "" {
		return fmt.Errorf("validate functional quarantine: entries[%d].package is required", index)
	}
	if _, known := knownPackages[entry.Package]; !known {
		return fmt.Errorf("validate functional quarantine: selector package %q is not discoverable in the functional package set", entry.Package)
	}
	if entry.Test != "" {
		if !functionalTestNamePattern.MatchString(entry.Test) {
			return fmt.Errorf("validate functional quarantine: selector %q has invalid top-level test name", functionalSelectorDisplay(entry))
		}
		if !slices.Contains(tests[entry.Package], entry.Test) {
			return fmt.Errorf("validate functional quarantine: selector %q is not discoverable in package %q", entry.Test, entry.Package)
		}
	}
	if entry.Bucket != functionalBucketEnvironment && entry.Bucket != functionalBucketFailure {
		return fmt.Errorf("validate functional quarantine: selector %q has unsupported bucket %q; expected %s or %s", functionalSelectorDisplay(entry), entry.Bucket, functionalBucketEnvironment, functionalBucketFailure)
	}
	if _, err := functionalQuarantineMeasurement(entry); err != nil {
		return fmt.Errorf("validate functional quarantine: selector %q has invalid measurement metadata: %w", functionalSelectorDisplay(entry), err)
	}
	if strings.TrimSpace(entry.Reason) == "" {
		return fmt.Errorf("validate functional quarantine: selector %q requires a non-empty reason", functionalSelectorDisplay(entry))
	}
	if entry.Bucket == functionalBucketFailure && strings.TrimSpace(entry.FollowUp) == "" {
		return fmt.Errorf("validate functional quarantine: genuinely failing selector %q requires a non-empty followUp", functionalSelectorDisplay(entry))
	}
	return nil
}

func buildFunctionalCoverageSelection(manifest functionalQuarantine, inventory functionalTestInventory) (functionalCoverageSelection, error) {
	packageExcluded := make(map[string]struct{})
	testExcluded := make(map[string]map[string]struct{})
	for _, entry := range manifest.Entries {
		if entry.Test == "" {
			packageExcluded[entry.Package] = struct{}{}
			continue
		}
		if testExcluded[entry.Package] == nil {
			testExcluded[entry.Package] = make(map[string]struct{})
		}
		testExcluded[entry.Package][entry.Test] = struct{}{}
	}

	selection := functionalCoverageSelection{
		Inventory:                inventory,
		SelectedTests:            make(map[string][]string),
		QuarantinedPackageCount:  len(packageExcluded),
		QuarantinedTestSelectors: len(manifest.Entries) - len(packageExcluded),
	}
	groups := make(map[string][]string)
	for _, packagePath := range inventory.Packages {
		if _, excluded := packageExcluded[packagePath]; excluded {
			selection.PackageExcludedTestCount += len(inventory.Tests[packagePath])
			continue
		}
		excludedTests := testExcluded[packagePath]
		selectedTests := make([]string, 0, len(inventory.Tests[packagePath]))
		for _, testName := range inventory.Tests[packagePath] {
			if _, excluded := excludedTests[testName]; excluded {
				continue
			}
			selectedTests = append(selectedTests, testName)
		}
		if len(inventory.Tests[packagePath]) > 0 && len(selectedTests) == 0 {
			selection.TestExcludedPackageCount++
			continue
		}
		selection.SelectedTests[packagePath] = append([]string(nil), selectedTests...)
		pattern := ""
		if len(excludedTests) > 0 {
			pattern = exactFunctionalTestRunPattern(selectedTests)
		}
		groups[pattern] = append(groups[pattern], packagePath)
		selection.SelectedTestCount += len(selectedTests)
	}

	patterns := make([]string, 0, len(groups))
	for pattern := range groups {
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	for _, pattern := range patterns {
		packagePaths := sortedUniqueStrings(groups[pattern])
		selection.Groups = append(selection.Groups, coverageRunGroup{Packages: packagePaths, RunPattern: pattern})
		selection.SelectedPackageCount += len(packagePaths)
	}
	if len(selection.Groups) == 0 {
		return functionalCoverageSelection{}, errors.New("build functional coverage selection: quarantine removes every runnable functional test; retain at least one selected test")
	}
	return selection, nil
}

func functionalTestCount(inventory functionalTestInventory) int {
	count := 0
	for _, tests := range inventory.Tests {
		count += len(tests)
	}
	return count
}

func selectedFunctionalPackages(selection functionalCoverageSelection) []string {
	packages := make([]string, 0, selection.SelectedPackageCount)
	for _, group := range selection.Groups {
		packages = append(packages, group.Packages...)
	}
	return sortedUniqueStrings(packages)
}

func selectedFunctionalTestInventory(selection functionalCoverageSelection) functionalTestInventory {
	packages := selectedFunctionalPackages(selection)
	tests := make(map[string][]string, len(packages))
	for _, packagePath := range packages {
		tests[packagePath] = append([]string(nil), selection.SelectedTests[packagePath]...)
	}
	return functionalTestInventory{Packages: packages, Tests: tests}
}

func exactFunctionalTestRunPattern(testNames []string) string {
	quoted := make([]string, 0, len(testNames))
	for _, name := range sortedUniqueStrings(testNames) {
		quoted = append(quoted, regexp.QuoteMeta(name))
	}
	return "^(?:" + strings.Join(quoted, "|") + ")$"
}

func functionalSelectorKey(packagePath, testName string) string {
	return packagePath + "\x00" + testName
}

func functionalSelectorDisplay(entry functionalQuarantineEntry) string {
	if entry.Test == "" {
		return entry.Package
	}
	return entry.Package + "#" + entry.Test
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
