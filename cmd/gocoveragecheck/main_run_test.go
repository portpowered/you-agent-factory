package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestExecuteReportsPassingCoverage(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	wantSuccess := "Go coverage 82.5% meets minimum 80.0%."
	if !strings.Contains(got, wantSuccess) {
		t.Fatalf("execute() stdout = %q, want success message %q", got, wantSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenCoverageBelowMinimum(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 90,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage 82.5% is below minimum 90.0%"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenCoverageBelowMinimumAndZeroCoveragePackage(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 90,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	wantFailure := strings.Join([]string{
		"go coverage 82.5% is below minimum 90.0%",
		"go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config",
	}, "\n")
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenZeroCoveragePackageOnly(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestRunCreatesAndRemovesTempCoverageProfile(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandWithTempProfileReport
	stdoutWriter = &stdout
	stderrWriter = &stderr

	result, err := run(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if result.actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", result.actual)
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty stderr", stderr.String())
	}

	profilePath := parseTempProfilePath(t, stdout.String())
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("temp profile %q still exists after run(), stat err = %v", profilePath, err)
	}
	if !strings.Contains(stdout.String(), "total: (statements) 82.5%") {
		t.Fatalf("run() stdout = %q, want total coverage line", stdout.String())
	}
}

func TestRunWrapsCoverSummaryFailureUsingStderrDetail(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandCoverFailsWithStderr
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
		profile:  filepath.Join(t.TempDir(), "coverage.out"),
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "summarize go coverage: exit status 3") {
		t.Fatalf("run() error = %q, want summarize wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stderr detail from cover tool") {
		t.Fatalf("run() error = %q, want stderr detail", err.Error())
	}
	if strings.Contains(err.Error(), "stdout detail from cover tool") {
		t.Fatalf("run() error = %q, did not expect stdout fallback detail", err.Error())
	}
}

func TestRunWrapsCoverSummaryFailureUsingStdoutFallback(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandCoverFailsWithStdout
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
		profile:  filepath.Join(t.TempDir(), "coverage.out"),
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "summarize go coverage: exit status 4") {
		t.Fatalf("run() error = %q, want summarize wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stdout detail from cover tool") {
		t.Fatalf("run() error = %q, want stdout fallback detail", err.Error())
	}
	if strings.Contains(err.Error(), "stderr detail from cover tool") {
		t.Fatalf("run() error = %q, did not expect stderr detail", err.Error())
	}
}

func TestRunWrapsCoverageLaneFailure(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandTestFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
		profile:  filepath.Join(t.TempDir(), "coverage.out"),
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	want := "run go test coverage lane: exit status 7"
	if err.Error() != want {
		t.Fatalf("run() error = %q, want %q", err.Error(), want)
	}
}

func TestRunWrapsCoverSummaryFailureWithoutDetail(t *testing.T) {
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCommand = fakeGoCoverageCommandCoverFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
		profile:  filepath.Join(t.TempDir(), "coverage.out"),
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	want := "summarize go coverage: exit status 8"
	if err.Error() != want {
		t.Fatalf("run() error = %q, want %q", err.Error(), want)
	}
}

func TestListGoPackagesWrapsListFailureUsingStderrDetail(t *testing.T) {
	originalExecCommand := execCommand
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoListCommandFailsWithStderr

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "list go packages: exit status 5") {
		t.Fatalf("listGoPackages() error = %q, want wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stderr detail from go list") {
		t.Fatalf("listGoPackages() error = %q, want stderr detail", err.Error())
	}
	if strings.Contains(err.Error(), "stdout detail from go list") {
		t.Fatalf("listGoPackages() error = %q, did not expect stdout fallback detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureUsingStdoutFallback(t *testing.T) {
	originalExecCommand := execCommand
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoListCommandFailsWithStdout

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "list go packages: exit status 6") {
		t.Fatalf("listGoPackages() error = %q, want wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stdout detail from go list") {
		t.Fatalf("listGoPackages() error = %q, want stdout fallback detail", err.Error())
	}
	if strings.Contains(err.Error(), "stderr detail from go list") {
		t.Fatalf("listGoPackages() error = %q, did not expect stderr detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureWithoutDetail(t *testing.T) {
	originalExecCommand := execCommand
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoListCommandFailsWithoutDetail

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	want := "list go packages: exit status 9"
	if err.Error() != want {
		t.Fatalf("listGoPackages() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenDefaultCoverageDiscoveryMatchesNoBackendPackages(t *testing.T) {
	originalExecCommand := execCommand
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoListCommandWithExcludedPackagesOnly

	_, _, err := resolveCoverageLane(config{})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenDefaultTestDiscoveryMatchesNoBackendPackages(t *testing.T) {
	originalExecCommand := execCommand
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoListCommandWithCoverageButNoTestPackages

	_, _, err := resolveCoverageLane(config{})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestListGoPackagesFiltersDuplicatesAndExcludedPackages(t *testing.T) {
	originalExecCommand := execCommand
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		execCommand = originalExecCommand
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nestedDir := filepath.Join(repoRoot, "cmd", "gocoveragecheck")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	execCommand = fakeGoListCommandWithDuplicatesAndExcludedPackages

	packages, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage)
	if err != nil {
		t.Fatalf("listGoPackages() error = %v", err)
	}

	want := []string{
		modulePath + "/cmd/factory",
		modulePath + "/pkg/config",
	}
	if !slices.Equal(packages, want) {
		t.Fatalf("listGoPackages() = %v, want %v", packages, want)
	}
}

func TestRepoRootDirFindsNearestAncestorWithGoMod(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nestedDir := filepath.Join(repoRoot, "pkg", "service", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	got, err := repoRootDir()
	if err != nil {
		t.Fatalf("repoRootDir() error = %v", err)
	}
	if testutil.CanonicalPath(got) != testutil.CanonicalPath(repoRoot) {
		t.Fatalf("repoRootDir() = %q, want %q", got, repoRoot)
	}
}

func TestRepoRootDirFailsWhenNoGoModExists(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	workingDir := filepath.Join(t.TempDir(), "pkg", "service")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	_, err = repoRootDir()
	if err == nil {
		t.Fatal("repoRootDir() unexpectedly succeeded")
	}
	if err.Error() != "resolve repository root: go.mod not found" {
		t.Fatalf("repoRootDir() error = %q, want missing go.mod error", err.Error())
	}
}

func TestFindZeroCoveragePackagesSkipsPackagesWithZeroStatements(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 0 0",
		"",
	}, "\n"))

	zeroCoveragePackages, err := findZeroCoveragePackages(
		modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/config",
			modulePath + "/pkg/generatedclient",
		},
	)
	if err != nil {
		t.Fatalf("findZeroCoveragePackages() error = %v", err)
	}
	if len(zeroCoveragePackages) != 0 {
		t.Fatalf("findZeroCoveragePackages() = %v, want none", zeroCoveragePackages)
	}
}
