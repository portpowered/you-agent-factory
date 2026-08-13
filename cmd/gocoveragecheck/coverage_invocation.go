package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// merges with mergeCoverageProfiles after all subprocesses succeed.
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
