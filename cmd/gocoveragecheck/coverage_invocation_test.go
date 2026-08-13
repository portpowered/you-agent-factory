package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBuildCoverageInvocationPlanBatchesOversizedWindowsCommand(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-short",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := make([]string, 0, 500)
	for index := 0; index < 500; index++ {
		testPackages = append(testPackages, modulePath+"/pkg/coverage_test_"+strings.Repeat("x", 32)+strconv.Itoa(index))
	}
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	legacyArgs := buildCoverageTestArgs(commonArgs, profilePath, false, testPackages)
	if got := windowsCommandLine(legacyArgs); got <= windowsCoverageCommandLineLimit {
		t.Fatalf("old single invocation length = %d, want it above safe limit %d", got, windowsCoverageCommandLineLimit)
	}

	plan, err := buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, false, "windows")
	if err != nil {
		t.Fatalf("buildCoverageInvocationPlan() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plan.cleanup(); cleanupErr != nil {
			t.Fatalf("cleanup batch profiles: %v", cleanupErr)
		}
	})

	if len(plan.invocations) < 2 {
		t.Fatalf("planned invocations = %d, want batching for oversized command", len(plan.invocations))
	}
	if len(plan.profilePaths) != len(plan.invocations) {
		t.Fatalf("profile paths = %d, invocations = %d, want one profile per invocation", len(plan.profilePaths), len(plan.invocations))
	}

	seen := make(map[string]int, len(testPackages))
	for index, invocation := range plan.invocations {
		if got := windowsCommandLine(invocation.args); got > windowsCoverageCommandLineLimit {
			t.Fatalf("invocation %d length = %d, want <= safe limit %d", index, got, windowsCoverageCommandLineLimit)
		}
		batchProfile := helperCoverProfilePath(invocation.args)
		if batchProfile == "" || batchProfile == profilePath {
			t.Fatalf("invocation %d profile = %q, want isolated batch profile", index, batchProfile)
		}
		if got := plan.profilePaths[index]; got != batchProfile {
			t.Fatalf("profilePaths[%d] = %q, invocation profile = %q", index, got, batchProfile)
		}
		for _, testPackage := range testPackages {
			if slicesContains(invocation.args, testPackage) {
				seen[testPackage]++
			}
		}
	}

	if len(seen) != len(testPackages) {
		t.Fatalf("planned package count = %d, want %d", len(seen), len(testPackages))
	}
	for _, testPackage := range testPackages {
		if seen[testPackage] != 1 {
			t.Fatalf("test package %q appears %d times, want exactly once", testPackage, seen[testPackage])
		}
	}

	batchDir := filepath.Dir(plan.profilePaths[0])
	if _, err := os.Stat(batchDir); err != nil {
		t.Fatalf("batch directory stat error = %v, want it to exist before cleanup", err)
	}
	if err := plan.cleanup(); err != nil {
		t.Fatalf("plan cleanup error = %v", err)
	}
	if _, err := os.Stat(batchDir); !os.IsNotExist(err) {
		t.Fatalf("batch directory still exists after cleanup, stat error = %v", err)
	}
	plan.cleanup = func() error { return nil }
}

func TestBuildCoverageInvocationPlanRejectsSingleOversizedWindowsPackage(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + strings.Repeat("example.com/backend/", 1200),
		"-p=2",
		"-count=1",
	}
	testPackage := modulePath + "/pkg/one-oversized-package"
	profilePath := filepath.Join(t.TempDir(), "coverage.out")

	_, err := buildCoverageInvocationPlan(commonArgs, []string{testPackage}, profilePath, false, "windows")
	if err == nil {
		t.Fatal("buildCoverageInvocationPlan() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "above the safe Windows limit") || !strings.Contains(err.Error(), testPackage) {
		t.Fatalf("buildCoverageInvocationPlan() error = %q, want package and bound diagnostic", err)
	}
}

func TestRunForOSBatchesAndMergesCoverageProfiles(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutWriter = &stdout
	stderrWriter = &stderr

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	testPackages := makeOversizedTestPackages(500)
	profilePath := filepath.Join(t.TempDir(), "merged-coverage.out")
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "test" {
			return "", "", errors.New("unexpected command in batched coverage test")
		}
		if got := windowsCommandLine(invocation.args); got > windowsCoverageCommandLineLimit {
			return "", "", errors.New("batched coverage command exceeded safe Windows limit")
		}
		invocations = append(invocations, invocation)
		batchIndex := len(invocations)
		coverage := strings.Join([]string{
			"mode: count",
			configPackage + "/config.go:1.1,2.1 3 " + strconv.Itoa(batchIndex-1),
			servicePackage + "/factory.go:1.1,2.1 5 0",
			"",
		}, "\n")
		return "", "", writeFakeCoverageProfile(helperCoverProfilePath(invocation.args), coverage)
	}

	result, err := runForOS(config{
		min:       0,
		totalOnly: true,
		coverpkg:  strings.Join([]string{configPackage, servicePackage}, ","),
		packages:  strings.Join(testPackages, " "),
		profile:   profilePath,
	}, "windows")
	if err != nil {
		t.Fatalf("runForOS() error = %v", err)
	}
	if len(invocations) < 2 {
		t.Fatalf("coverage invocations = %d, want multiple Windows batches", len(invocations))
	}
	if result.actual != 37.5 {
		t.Fatalf("merged coverage = %v, want 37.5", result.actual)
	}
	if result.packageTotals[configPackage] != (packageCoverageTotals{coveredStatements: 3, totalStatements: 3}) {
		t.Fatalf("config totals = %+v, want 3/3", result.packageTotals[configPackage])
	}
	if result.packageTotals[servicePackage] != (packageCoverageTotals{coveredStatements: 0, totalStatements: 5}) {
		t.Fatalf("service totals = %+v, want 0/5", result.packageTotals[servicePackage])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("merged profile stat error = %v", err)
	}
	for _, invocation := range invocations {
		batchProfile := helperCoverProfilePath(invocation.args)
		if _, err := os.Stat(filepath.Dir(batchProfile)); !os.IsNotExist(err) {
			t.Fatalf("batch directory for %q still exists, stat error = %v", batchProfile, err)
		}
	}
}

func makeOversizedTestPackages(count int) []string {
	packages := make([]string, 0, count)
	for index := 0; index < count; index++ {
		packages = append(packages, modulePath+"/pkg/coverage_test_"+strings.Repeat("x", 32)+strconv.Itoa(index))
	}
	return packages
}
