package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func buildCoverageTestArgs(commonArgs []string, profilePath string, timingEnabled bool, testPackages []string) []string {
	args := append([]string(nil), commonArgs...)
	args = append(args, fmt.Sprintf("-coverprofile=%s", profilePath))
	if timingEnabled {
		args = append(args, "-json")
	}
	return append(args, testPackages...)
}

// buildCoverageInvocationPlan keeps the ordinary one-process invocation on
// every platform unless a Windows command would exceed the conservative bound.
// Windows batches retain all fixed go test flags and execute each resolved test
// package exactly once. Each batch writes an isolated profile that the caller
// merges with mergeCoverageProfiles after execution, including profiles from
// failed subprocesses when they were produced.
func buildCoverageInvocationPlan(commonArgs []string, testPackages []string, profilePath string, timingEnabled bool, targetOS string) (coverageInvocationPlan, error) {
	directArgs := buildCoverageTestArgs(commonArgs, profilePath, timingEnabled, testPackages)
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
		candidateArgs := buildCoverageTestArgs(commonArgs, candidateProfile, timingEnabled, candidateBatch)
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

		appendCoverageInvocation(&plan, commonArgs, candidateProfile, timingEnabled, currentBatch)
		currentBatch = []string{testPackage}
		candidateProfile = batchProfilePath(batchDir, len(plan.invocations))
		candidateArgs = buildCoverageTestArgs(commonArgs, candidateProfile, timingEnabled, currentBatch)
		if !coverageCommandFitsWindowsLimit(candidateArgs) {
			return coverageInvocationPlan{}, errors.Join(
				coverageCommandTooLongError(testPackage, candidateArgs),
				cleanupBatchDir(),
			)
		}
	}

	if len(currentBatch) > 0 {
		appendCoverageInvocation(&plan, commonArgs, batchProfilePath(batchDir, len(plan.invocations)), timingEnabled, currentBatch)
	}
	return plan, nil
}

func appendCoverageInvocation(plan *coverageInvocationPlan, commonArgs []string, profilePath string, timingEnabled bool, testPackages []string) {
	plan.profilePaths = append(plan.profilePaths, profilePath)
	plan.invocations = append(plan.invocations, commandInvocation{
		name: "go",
		args: buildCoverageTestArgs(commonArgs, profilePath, timingEnabled, testPackages),
		env:  os.Environ(),
	})
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

// runGoTestCoverageLane runs the coverage invocation once on short command
// lines, or in deterministic test-package batches when Windows needs it. When
// cfg.timingOutput is set, it combines every batch's -json stdout before
// writing the timing summary, so a failed or crashed lane still leaves
// trustworthy (possibly incomplete) diagnostics on disk.
func runGoTestCoverageLane(cfg config, commonArgs []string, testPackages []string, profilePath string, repoRoot string, coverPackages []string, targetOS string, failurePrefix string) error {
	timingEnabled := strings.TrimSpace(cfg.timingOutput) != ""
	plan, err := buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, timingEnabled, targetOS)
	if err != nil {
		return err
	}

	started := time.Now()
	var stdout strings.Builder
	var stderr strings.Builder
	var laneErr error
	succeeded := make([]bool, len(plan.invocations))
	for index, invocation := range plan.invocations {
		batchStdout, batchStderr, commandErr := runCommand(invocation)
		appendCoverageOutput(&stdout, batchStdout)
		appendCoverageOutput(&stderr, batchStderr)
		if commandErr == nil {
			succeeded[index] = true
			continue
		}

		detail := mergeGoTestFailureDetail(batchStderr, batchStdout)
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

	var timingWriteErr error
	if timingEnabled {
		summary := buildFunctionalTimingSummary(stdout.String(), testPackages, wallSeconds)
		timingWriteErr = writeFunctionalTimingSummaryJSON(cfg.timingOutput, summary)
	}

	var mergeErr error
	if len(plan.profilePaths) > 1 {
		availableProfiles, availabilityErr := availableBatchCoverageProfiles(plan.profilePaths, succeeded)
		mergeErr = availabilityErr
		if len(availableProfiles) > 0 {
			mergeErr = errors.Join(mergeErr, mergeCoverageProfiles(availableProfiles, profilePath, repoRoot, coverPackages))
		}
	}

	return errors.Join(laneErr, timingWriteErr, mergeErr, plan.cleanup())
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
