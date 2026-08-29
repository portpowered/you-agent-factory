package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Windows CreateProcess accepts a command line of at most 32,767 UTF-16 code
// units. Keep coverage commands well below that limit because the final
// executable path and escaped arguments are not known until process launch.
// The length calculation below is intentionally an upper bound, not a claim
// about the exact command-line encoding used by os/exec.
const windowsCoverageCommandLineLimit = 24_000

type coverageInvocationPlan struct {
	invocations  []commandInvocation
	profilePaths []string
	cleanup      func() error
}

type coverageTestFailureError struct {
	err                  error
	failedTestCount      int
	failedTestCountKnown bool
}

func (err *coverageTestFailureError) Error() string {
	return err.err.Error()
}

func (err *coverageTestFailureError) Unwrap() error {
	return err.err
}

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

func buildCoverageTestArgs(commonArgs []string, profilePath string, jsonEventsEnabled bool, testPackages []string) []string {
	args := append([]string(nil), commonArgs...)
	args = append(args, fmt.Sprintf("-coverprofile=%s", profilePath))
	if jsonEventsEnabled {
		args = append(args, "-json")
	}
	return append(args, testPackages...)
}

// buildCoverageInvocationPlan keeps the ordinary one-process invocation on
// every platform unless a Windows command would exceed the conservative bound.
// A compactPackageArgs set is used only for that direct invocation; if it does
// not fit, Windows batches retain all fixed go test flags and execute each
// resolved test package exactly once. Each batch writes an isolated profile
// that the caller merges with mergeCoverageProfiles after execution, including
// profiles from failed subprocesses when they were produced.
func buildCoverageInvocationPlan(commonArgs []string, testPackages []string, profilePath string, jsonEventsEnabled bool, targetOS string, compactPackageArgs ...[]string) (coverageInvocationPlan, error) {
	directPackageArgs := testPackages
	if len(compactPackageArgs) > 0 && len(compactPackageArgs[0]) > 0 {
		directPackageArgs = compactPackageArgs[0]
	}
	directArgs := buildCoverageTestArgs(commonArgs, profilePath, jsonEventsEnabled, directPackageArgs)
	if targetOS != "windows" || coverageCommandFitsWindowsLimit(directArgs) {
		return coverageInvocationPlan{
			invocations: []commandInvocation{{
				name: "go",
				args: directArgs,
				env:  os.Environ(),
			}},
			profilePaths: []string{profilePath},
			cleanup:      func() error { return nil },
		}, nil
	}

	if len(testPackages) == 0 {
		return coverageInvocationPlan{}, fmt.Errorf(
			"prepare go coverage lane: command line is %d characters, above the safe Windows limit of %d, and no test packages are available to batch",
			windowsCommandLine(directArgs), windowsCoverageCommandLineLimit,
		)
	}

	batchDir, err := os.MkdirTemp("", "gocoveragecheck-batches-*")
	if err != nil {
		return coverageInvocationPlan{}, fmt.Errorf("create temporary go coverage batch directory: %w", err)
	}
	cleanupBatchDir := func() error {
		return os.RemoveAll(batchDir)
	}

	plan := coverageInvocationPlan{cleanup: cleanupBatchDir}
	currentBatch := make([]string, 0)
	for _, testPackage := range testPackages {
		candidateBatch := append(append([]string(nil), currentBatch...), testPackage)
		candidateProfile := batchProfilePath(batchDir, len(plan.invocations))
		candidateArgs := buildCoverageTestArgs(commonArgs, candidateProfile, jsonEventsEnabled, candidateBatch)
		if coverageCommandFitsWindowsLimit(candidateArgs) {
			currentBatch = candidateBatch
			continue
		}

		if len(currentBatch) == 0 {
			return coverageInvocationPlan{}, errors.Join(
				coverageCommandTooLongError(testPackage, candidateArgs),
				cleanupBatchDir(),
			)
		}

		appendCoverageInvocation(&plan, commonArgs, candidateProfile, jsonEventsEnabled, currentBatch)
		currentBatch = []string{testPackage}
		candidateProfile = batchProfilePath(batchDir, len(plan.invocations))
		candidateArgs = buildCoverageTestArgs(commonArgs, candidateProfile, jsonEventsEnabled, currentBatch)
		if !coverageCommandFitsWindowsLimit(candidateArgs) {
			return coverageInvocationPlan{}, errors.Join(
				coverageCommandTooLongError(testPackage, candidateArgs),
				cleanupBatchDir(),
			)
		}
	}

	if len(currentBatch) > 0 {
		appendCoverageInvocation(&plan, commonArgs, batchProfilePath(batchDir, len(plan.invocations)), jsonEventsEnabled, currentBatch)
	}
	return plan, nil
}

func appendCoverageInvocation(plan *coverageInvocationPlan, commonArgs []string, profilePath string, jsonEventsEnabled bool, testPackages []string) {
	plan.profilePaths = append(plan.profilePaths, profilePath)
	plan.invocations = append(plan.invocations, commandInvocation{
		name: "go",
		args: buildCoverageTestArgs(commonArgs, profilePath, jsonEventsEnabled, testPackages),
		env:  os.Environ(),
	})
}

// compactGoPackagePatterns returns disjoint Go package patterns whose expansion
// is exactly selectedPackages. The allPackages input is the package universe
// used to prove that a pattern does not pull an excluded lane into go test.
func compactGoPackagePatterns(allPackages []string, selectedPackages []string, packageRoot string) ([]string, error) {
	allSet, selectedSet, err := buildGoPackageSets(allPackages, selectedPackages, packageRoot)
	if err != nil {
		return nil, err
	}
	patterns := collectCompactGoPackagePatterns(allSet, selectedSet, packageRoot)
	if err := validateCompactGoPackagePatterns(patterns, allSet, selectedSet); err != nil {
		return nil, err
	}
	return patterns, nil
}

func buildGoPackageSets(allPackages []string, selectedPackages []string, packageRoot string) (map[string]struct{}, map[string]struct{}, error) {
	if strings.TrimSpace(packageRoot) == "" {
		return nil, nil, errors.New("compact go package patterns: package root is empty")
	}

	allSet := make(map[string]struct{}, len(allPackages))
	for _, packagePath := range allPackages {
		packagePath = strings.TrimSpace(packagePath)
		if packagePath != "" {
			allSet[packagePath] = struct{}{}
		}
	}
	selectedSet := make(map[string]struct{}, len(selectedPackages))
	for _, packagePath := range selectedPackages {
		packagePath = strings.TrimSpace(packagePath)
		if packagePath == "" {
			continue
		}
		if !strings.HasPrefix(packagePath, packageRoot) || (len(packagePath) > len(packageRoot) && packagePath[len(packageRoot)] != '/') {
			return nil, nil, fmt.Errorf("compact go package patterns: selected package %q is outside root %q", packagePath, packageRoot)
		}
		if _, ok := allSet[packagePath]; !ok {
			return nil, nil, fmt.Errorf("compact go package patterns: selected package %q is missing from package universe", packagePath)
		}
		selectedSet[packagePath] = struct{}{}
	}
	if len(selectedSet) == 0 {
		return nil, nil, errors.New("compact go package patterns: no selected packages")
	}
	return allSet, selectedSet, nil
}

func collectCompactGoPackagePatterns(allSet map[string]struct{}, selectedSet map[string]struct{}, packageRoot string) []string {
	patterns := make([]string, 0)
	var visit func(string)
	visit = func(prefix string) {
		descendants := make([]string, 0)
		for packagePath := range allSet {
			if packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/") {
				descendants = append(descendants, packagePath)
			}
		}
		if len(descendants) == 0 {
			return
		}
		allSelected := true
		for _, packagePath := range descendants {
			if _, ok := selectedSet[packagePath]; !ok {
				allSelected = false
				break
			}
		}
		if allSelected {
			patterns = append(patterns, prefix+"/...")
			return
		}
		if _, ok := selectedSet[prefix]; ok {
			patterns = append(patterns, prefix)
		}

		children := make(map[string]struct{})
		for _, packagePath := range descendants {
			if packagePath == prefix {
				continue
			}
			rest := strings.TrimPrefix(packagePath, prefix+"/")
			child := strings.SplitN(rest, "/", 2)[0]
			children[child] = struct{}{}
		}
		childNames := make([]string, 0, len(children))
		for child := range children {
			childNames = append(childNames, child)
		}
		sort.Strings(childNames)
		for _, child := range childNames {
			visit(prefix + "/" + child)
		}
	}
	visit(packageRoot)
	sort.Strings(patterns)
	return patterns
}

func validateCompactGoPackagePatterns(patterns []string, allSet map[string]struct{}, selectedSet map[string]struct{}) error {
	matched := make(map[string]struct{}, len(selectedSet))
	for _, pattern := range patterns {
		subtree := strings.HasSuffix(pattern, "/...")
		prefix := strings.TrimSuffix(pattern, "/...")
		for packagePath := range allSet {
			if packagePath == prefix || (subtree && strings.HasPrefix(packagePath, prefix+"/")) {
				matched[packagePath] = struct{}{}
			}
		}
	}
	if len(matched) != len(selectedSet) {
		return fmt.Errorf("compact go package patterns: selected %d packages but patterns match %d (%v)", len(selectedSet), len(matched), patterns)
	}
	for packagePath := range matched {
		if _, ok := selectedSet[packagePath]; !ok {
			return fmt.Errorf("compact go package patterns: pattern set includes unselected package %q", packagePath)
		}
	}
	return nil
}

func batchProfilePath(batchDir string, index int) string {
	return filepath.Join(batchDir, fmt.Sprintf("coverage-%06d.out", index))
}

func coverageCommandFitsWindowsLimit(args []string) bool {
	return windowsCommandLine(args) <= windowsCoverageCommandLineLimit
}

func coverageCommandTooLongError(testPackage string, args []string) error {
	return fmt.Errorf(
		"prepare go coverage lane: command for test package %q is %d characters, above the safe Windows limit of %d; shorten the coverage package or test package arguments",
		testPackage, windowsCommandLine(args), windowsCoverageCommandLineLimit,
	)
}

func windowsCommandLine(args []string) int {
	commandName := "go"
	if resolved, err := exec.LookPath(commandName); err == nil {
		commandName = resolved
	}

	length := windowsCommandLineArgLength(commandName)
	for _, arg := range args {
		length++ // separator between command-line arguments
		length += windowsCommandLineArgLength(arg)
	}
	return length
}

// windowsCommandLineArgLength is deliberately conservative. It charges every
// argument for quotes and escaped backslashes, even when the actual argument
// would not need quoting, and uses UTF-8 byte length as an upper bound for the
// ASCII-heavy paths and package names in this command.
func windowsCommandLineArgLength(arg string) int {
	return 2 + len(arg) + strings.Count(arg, `\`) + 2*strings.Count(arg, `"`)
}

// planGoTestCoverageLane builds the coverage invocation once on short command
// lines, or in deterministic test-package batches when Windows needs it.
func planGoTestCoverageLane(commonArgs []string, testPackages []string, profilePath string, cfg config, targetOS string, compactPackageArgs ...[]string) (coverageInvocationPlan, error) {
	jsonEventsEnabled := strings.TrimSpace(cfg.timingOutput) != "" || cfg.stream
	return buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, jsonEventsEnabled, targetOS, compactPackageArgs...)
}

func planGoTestCoverageLaneWithSelection(commonArgs []string, profilePath string, cfg config, targetOS string, selection functionalCoverageSelection) (coverageInvocationPlan, error) {
	jsonEventsEnabled := strings.TrimSpace(cfg.timingOutput) != "" || cfg.stream
	return buildFunctionalCoverageInvocationPlan(commonArgs, selection.Groups, profilePath, jsonEventsEnabled, targetOS)
}

// runGoTestCoverageLane runs the coverage invocation once on short command
// lines, or in deterministic test-package batches when Windows needs it. When
// timing capture or streaming is enabled, it combines every batch's -json
// stdout before writing the timing summary, so a failed or crashed lane still
// leaves trustworthy (possibly incomplete) diagnostics on disk.
func runGoTestCoverageLane(cfg config, commonArgs []string, testPackages []string, profilePath string, repoRoot string, coverPackages []string, targetOS string, failurePrefix string, compactPackageArgs ...[]string) error {
	plan, err := planGoTestCoverageLane(commonArgs, testPackages, profilePath, cfg, targetOS, compactPackageArgs...)
	if err != nil {
		return err
	}
	return executeCoverageInvocationPlan(cfg, plan, testPackages, profilePath, repoRoot, coverPackages, failurePrefix, nil)
}

func executeCoverageInvocationPlan(cfg config, plan coverageInvocationPlan, testPackages []string, profilePath string, repoRoot string, coverPackages []string, failurePrefix string, expectedFunctionalInventory *functionalTestInventory) error {
	buildDiagnosticRun, probeErr := runCoverageBuildProbe(cfg, plan, testPackages)
	if probeErr != nil {
		return errors.Join(probeErr, plan.cleanup())
	}

	started := time.Now()
	snapshotter := configureFunctionalTimingSnapshot(&plan, cfg, testPackages, started, profilePath, repoRoot, coverPackages)
	if snapshotter != nil {
		defer snapshotter.stopAndWait()
	}
	var stdout strings.Builder
	var stderr strings.Builder
	var laneErr error
	succeeded := make([]bool, len(plan.invocations))
	testFailureObserved := false
	failedTestCount := 0
	failedTestCountKnown := true
	for index, invocation := range plan.invocations {
		batchStdout, batchStderr, commandErr := runCommand(invocation)
		commandErr = errors.Join(commandErr, flushFunctionalStreamWriter(invocation.stdoutWriter))
		appendCoverageOutput(&stdout, batchStdout)
		appendCoverageOutput(&stderr, batchStderr)
		if commandErr == nil {
			succeeded[index] = true
			continue
		}
		if count, known, observed := coverageTestFailureDetails(commandErr, batchStdout, batchStderr); observed {
			testFailureObserved = true
			failedTestCount += count
			failedTestCountKnown = failedTestCountKnown && known
		}

		detail := coverageFailureDetail(cfg, batchStdout, batchStderr)
		batchFailurePrefix := failurePrefix
		if len(plan.invocations) > 1 {
			batchFailurePrefix = fmt.Sprintf("%s (batch %d/%d)", failurePrefix, index+1, len(plan.invocations))
		}
		if detail != "" {
			laneErr = errors.Join(laneErr, fmt.Errorf("%s: %w\n%s", batchFailurePrefix, commandErr, detail))
		} else {
			laneErr = errors.Join(laneErr, fmt.Errorf("%s: %w", batchFailurePrefix, commandErr))
		}
	}
	wallSeconds := time.Since(started).Seconds()

	var buildDiagnosticErr error
	if buildDiagnosticRun != nil {
		buildDiagnosticErr = finalizeCoverageBuildDiagnostics(
			buildDiagnosticRun,
			strings.TrimSpace(cfg.coverageBuildDiagnosticsOutput),
			wallSeconds,
		)
	}

	timingWriteErr := finalizeFunctionalTiming(cfg, snapshotter, stdout.String(), testPackages, wallSeconds, laneErr, expectedFunctionalInventory)

	var mergeErr error
	if len(plan.profilePaths) > 1 {
		availableProfiles, availabilityErr := availableBatchCoverageProfiles(plan.profilePaths, succeeded)
		mergeErr = availabilityErr
		if len(availableProfiles) > 0 {
			mergeErr = errors.Join(mergeErr, mergeCoverageProfiles(availableProfiles, profilePath, repoRoot, coverPackages))
		}
	}

	partialCoverageErr := publishPartialCoverageIfNeeded(cfg, profilePath, repoRoot, coverPackages, laneErr, mergeErr)
	runErr := errors.Join(laneErr, timingWriteErr, buildDiagnosticErr, mergeErr, partialCoverageErr, plan.cleanup())
	if !testFailureObserved {
		return runErr
	}
	return &coverageTestFailureError{
		err:                  runErr,
		failedTestCount:      failedTestCount,
		failedTestCountKnown: failedTestCountKnown && failedTestCount > 0,
	}
}

func configureFunctionalTimingSnapshot(plan *coverageInvocationPlan, cfg config, testPackages []string, started time.Time, profilePath string, repoRoot string, coverPackages []string) *functionalTimingSnapshotter {
	timingEnabled := strings.TrimSpace(cfg.timingOutput) != ""
	if !cfg.stream && !timingEnabled {
		return nil
	}

	tracker := newFunctionalTimingTracker(testPackages, started)
	var snapshotter *functionalTimingSnapshotter
	observer := func(event goTestTimingEvent) {
		if snapshotter != nil {
			snapshotter.observe(event)
			return
		}
		tracker.observe(event)
	}
	var reporter *functionalStreamReporter
	if cfg.stream {
		reporter = configureCoverageInvocationStreamingForSuite(plan, true, cfg.suite == functionalCoverageSuite, observer)
	} else {
		reporter = configureCoverageInvocationObservation(plan, observer)
	}
	sink := io.Writer(nil)
	var sinkMu *sync.Mutex
	if cfg.stream && cfg.suite != functionalCoverageSuite {
		sink = stdoutWriter
		sinkMu = &reporter.sinkMu
	}
	snapshotter = newFunctionalTimingSnapshotter(
		tracker,
		cfg.timingOutput,
		sink,
		sinkMu,
		func() error {
			return writePartialCoverageSnapshot(
				cfg.jsonOutput,
				profilePath,
				repoRoot,
				coverPackages,
				cfg.packageFloorPolicyValue(),
				partialCoverageReason(nil),
			)
		},
	)
	snapshotter.publish(false, coverageLaneNoun(cfg.suite)+" run started", false)
	return snapshotter
}

func finalizeFunctionalTiming(cfg config, snapshotter *functionalTimingSnapshotter, stdout string, testPackages []string, wallSeconds float64, laneErr error, expectedFunctionalInventory *functionalTestInventory) error {
	runtimeInventoryGate := expectedFunctionalInventory != nil
	if strings.TrimSpace(cfg.timingOutput) == "" && !runtimeInventoryGate && cfg.suite != functionalCoverageSuite {
		if snapshotter != nil {
			snapshotter.stopAndWait()
		}
		return nil
	}

	summary := buildFunctionalTimingSummary(stdout, testPackages, wallSeconds)
	timingReason := ""
	if !summary.Complete {
		timingReason = coverageLaneNoun(cfg.suite) + " " + incompleteTimingCaptureCause(summary)
		if laneErr != nil {
			timingReason += ": " + compactDiagnosticError(laneErr)
		}
	}
	var inventoryErr error
	if runtimeInventoryGate {
		inventoryErr = validateFunctionalRuntimeInventory(stdout, *expectedFunctionalInventory)
		if inventoryErr != nil && timingReason == "" {
			timingReason = coverageLaneNoun(cfg.suite) + " runtime inventory validation failed: " + compactDiagnosticError(inventoryErr)
		}
	}
	var err error
	if snapshotter != nil {
		err = snapshotter.finish(summary, timingReason)
	} else {
		err = writeFunctionalTimingSummaryJSON(cfg.timingOutput, summary)
	}
	if err == nil && cfg.suite == functionalCoverageSuite {
		if summary.Complete {
			if reportErr := writeFunctionalTimingReport(summary); reportErr != nil {
				err = errors.Join(err, reportErr)
			}
		}
	}
	return errors.Join(err, inventoryErr)
}

// incompleteTimingCaptureCause names why a capture is partial. The two causes
// are unrelated and a reader acts on them differently, so reporting the wrong
// one is worse than reporting none: the unit lane's first hosted run recorded
// every one of its 548 packages and still described itself as having ended
// early, which sends a reader hunting a truncated run that never happened.
// Failing to parse a fraction of the child's output is the ordinary case at
// that size, where one unreadable line among hundreds of thousands is enough.
func incompleteTimingCaptureCause(summary functionalTimingSummaryJSON) string {
	if summary.PackageCount < summary.ExpectedPackageCount {
		return "timing capture ended before every package reported a terminal result"
	}
	return "timing capture could not read every go test event, so some per-test rows may be missing"
}

func publishPartialCoverageIfNeeded(cfg config, profilePath string, repoRoot string, coverPackages []string, laneErr error, mergeErr error) error {
	if laneErr == nil && mergeErr == nil {
		return nil
	}
	return writePartialCoverageSnapshot(
		cfg.jsonOutput,
		profilePath,
		repoRoot,
		coverPackages,
		cfg.packageFloorPolicyValue(),
		partialCoverageReason(errors.Join(laneErr, mergeErr)),
	)
}

func coverageTestFailureDetails(commandErr error, stdout string, stderr string) (int, bool, bool) {
	if !isCoverageTestProcessFailure(commandErr) {
		return 0, false, false
	}
	count := countObservedGoTestFailures(stdout, stderr)
	return count, count > 0, true
}

func isCoverageTestProcessFailure(commandErr error) bool {
	var exitErr *exec.ExitError
	if errors.As(commandErr, &exitErr) {
		return true
	}
	return strings.HasPrefix(commandErr.Error(), "exit status ")
}

func countObservedGoTestFailures(stdout string, stderr string) int {
	count := 0
	for _, output := range []string{stdout, stderr} {
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--- FAIL:") {
				count++
				continue
			}
			if !strings.HasPrefix(trimmed, "{") {
				continue
			}

			var event goTestTimingEvent
			if err := json.Unmarshal([]byte(trimmed), &event); err == nil && event.Action == timingOutcomeFail && event.Test != "" {
				count++
			}
		}
	}
	return count
}

func buildFunctionalCoverageInvocationPlan(commonArgs []string, groups []coverageRunGroup, profilePath string, jsonEventsEnabled bool, targetOS string) (coverageInvocationPlan, error) {
	if len(groups) == 0 {
		return coverageInvocationPlan{}, errors.New("prepare functional coverage lane: no selected run groups")
	}
	if len(groups) == 1 {
		args := appendCoverageRunPattern(commonArgs, groups[0].RunPattern)
		return buildCoverageInvocationPlan(args, groups[0].Packages, profilePath, jsonEventsEnabled, targetOS)
	}

	groupDir, err := os.MkdirTemp("", "gocoveragecheck-functional-groups-*")
	if err != nil {
		return coverageInvocationPlan{}, fmt.Errorf("create functional coverage group directory: %w", err)
	}
	cleanups := []func() error{func() error { return os.RemoveAll(groupDir) }}
	plan := coverageInvocationPlan{}
	for index, group := range groups {
		groupProfile := filepath.Join(groupDir, fmt.Sprintf("coverage-%06d.out", index))
		args := appendCoverageRunPattern(commonArgs, group.RunPattern)
		groupPlan, groupErr := buildCoverageInvocationPlan(args, group.Packages, groupProfile, jsonEventsEnabled, targetOS)
		if groupErr != nil {
			for _, cleanup := range cleanups {
				groupErr = errors.Join(groupErr, cleanup())
			}
			return coverageInvocationPlan{}, groupErr
		}
		plan.invocations = append(plan.invocations, groupPlan.invocations...)
		plan.profilePaths = append(plan.profilePaths, groupPlan.profilePaths...)
		cleanups = append(cleanups, groupPlan.cleanup)
	}
	plan.cleanup = func() error {
		var cleanupErr error
		for _, cleanup := range cleanups {
			cleanupErr = errors.Join(cleanupErr, cleanup())
		}
		return cleanupErr
	}
	return plan, nil
}

func appendCoverageRunPattern(commonArgs []string, pattern string) []string {
	if strings.TrimSpace(pattern) == "" {
		return append([]string(nil), commonArgs...)
	}
	args := append([]string(nil), commonArgs...)
	return append(args, "-run="+pattern)
}

func availableBatchCoverageProfiles(profilePaths []string, succeeded []bool) ([]string, error) {
	available := make([]string, 0, len(profilePaths))
	var profileErr error
	for index, profilePath := range profilePaths {
		_, err := os.Stat(profilePath)
		switch {
		case err == nil:
			available = append(available, profilePath)
		case os.IsNotExist(err) && !succeeded[index]:
			// A failed go test subprocess may not have written a profile.
		case os.IsNotExist(err):
			profileErr = errors.Join(profileErr, fmt.Errorf("go coverage batch %d completed without writing profile %q", index+1, profilePath))
		default:
			profileErr = errors.Join(profileErr, fmt.Errorf("inspect go coverage batch profile %q: %w", profilePath, err))
		}
	}
	return available, profileErr
}

func appendCoverageOutput(output *strings.Builder, chunk string) {
	if chunk == "" {
		return
	}
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
	output.WriteString(chunk)
}

func runCommand(invocation commandInvocation) (string, string, error) {
	return commandRunner(invocation)
}

func configureCoverageInvocationStreaming(plan *coverageInvocationPlan, enabled bool, observers ...func(goTestTimingEvent)) *functionalStreamReporter {
	return configureCoverageInvocationStreamingForSuite(plan, enabled, false, observers...)
}

func configureCoverageInvocationStreamingForSuite(plan *coverageInvocationPlan, enabled bool, suppressHumanOutput bool, observers ...func(goTestTimingEvent)) *functionalStreamReporter {
	if !enabled {
		return nil
	}
	var observer func(goTestTimingEvent)
	if len(observers) > 0 {
		observer = observers[0]
	}
	reporter := newFunctionalStreamReporterWithObserverMode(stdoutWriter, observer, suppressHumanOutput)
	for index := range plan.invocations {
		plan.invocations[index].stdoutWriter = reporter.stdoutWriter()
		stderrSink := io.Writer(stderrWriter)
		if suppressHumanOutput {
			stderrSink = io.Discard
		}
		plan.invocations[index].stderrWriter = reporter.stderrWriter(stderrSink)
	}
	return reporter
}

func configureCoverageInvocationObservation(plan *coverageInvocationPlan, observer func(goTestTimingEvent)) *functionalStreamReporter {
	reporter := newFunctionalStreamReporterWithObserver(io.Discard, observer)
	for index := range plan.invocations {
		plan.invocations[index].stdoutWriter = reporter.stdoutWriter()
		plan.invocations[index].stderrWriter = reporter.stderrWriter(io.Discard)
	}
	return reporter
}

// coverageFailureDetailStdout returns the child stdout a failing batch should
// be attributed with.
//
// Asking for a timing summary switches the child go test to -json, so the
// buffered stdout holds test2json events rather than the human text a reader
// needs to see which test failed and why. A lane that captures timing without
// streaming therefore renders the human output back out of those events, which
// keeps a failing unit lane exactly as diagnosable as it is with timing capture
// switched off.
//
// A non-quiet streaming lane already forwarded the human text to its sink line
// by line, so its buffer is returned unchanged. The functional lane uses a
// quiet streaming reporter and is rendered by coverageFailureDetail instead.
func coverageFailureDetailStdout(cfg config, stdout string) string {
	if cfg.stream || strings.TrimSpace(cfg.timingOutput) == "" {
		return stdout
	}
	return renderGoTestEventOutput(stdout)
}

func coverageFailureDetail(cfg config, stdout string, stderr string) string {
	if cfg.suite == functionalCoverageSuite && (cfg.stream || strings.TrimSpace(cfg.timingOutput) != "") {
		return mergeGoTestFailureDetail(stderr, renderFunctionalFailureDetail(stdout))
	}
	return mergeGoTestFailureDetail(stderr, coverageFailureDetailStdout(cfg, stdout))
}

// renderGoTestEventOutput reconstitutes the human-readable go test stream from
// captured test2json events. go test wraps every byte the test binary and the
// toolchain write in an "output" event, so replaying those events in order
// reproduces the original stream. Lines that are not decodable events pass
// through unchanged: a child that crashed before test2json framed its output
// still contributes its raw fragment.
func renderGoTestEventOutput(stdout string) string {
	if strings.TrimSpace(stdout) == "" {
		return stdout
	}
	var rendered strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event goTestTimingEvent
		if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil || event.Package == "" {
			rendered.Write(line)
			rendered.WriteString("\n")
			continue
		}
		if event.Action != "output" {
			continue
		}
		rendered.WriteString(event.Output)
	}
	return rendered.String()
}

func mergeGoTestFailureDetail(stderr string, stdout string) string {
	stderr = strings.TrimSpace(compactCoverageOutput(stderr))
	stdout = strings.TrimSpace(compactCoverageOutput(stdout))
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	case strings.Contains(stdout, "\nFAIL") || strings.Contains(stdout, "--- FAIL:"):
		return stdout + "\n" + stderr
	default:
		return stderr + "\n" + stdout
	}
}

func compactCoverageOutput(output string) string {
	return coveragePackageListPattern.ReplaceAllString(output, "$1")
}
