package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestExecuteReportsPassingCoverage(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
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
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	if strings.Contains(got, "\t\tcoverage: 75.0% of statements") {
		t.Fatalf("execute() stdout = %q, did not expect raw go test coverage output", got)
	}
	wantSuccess := "Go coverage 100.0% meets minimum 80.0%."
	if !strings.Contains(got, wantSuccess) {
		t.Fatalf("execute() stdout = %q, want success message %q", got, wantSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteTotalOnlyReportsIndependentSuiteCoverageWithPackageSummaries(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:       80,
		coverpkg:  modulePath + "/pkg/config",
		packages:  "./tests/functional/runtime_api",
		totalOnly: true,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary from the selected suite profile", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFunctionalFailsForNonBaselinedPackageBelowTarget(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		coverpkg:        modulePath + "/pkg/config",
		packages:        "./tests/functional/runtime_api",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
		packageMin:      80,
		suite:           "functional",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly accepted a new functional package below target")
	}
	want := "go coverage found non-baselined backend packages below 80.0% statement coverage: " +
		modulePath + "/pkg/config (0.0%)"
	if err.Error() != want {
		t.Fatalf("execute() error = %q, want %q", err.Error(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenCoverageBelowMinimum(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 100.1,
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
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := "go coverage 100.0% is below minimum 100.1%"
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

func TestExecuteEnforcesAggregateAndManifestMinimumsIndependently(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	tests := []struct {
		name         string
		minimum      string
		aggregateMin float64
		command      commandRunnerFunc
		wantFailure  string
		rejectText   string
	}{
		{
			name:         "aggregate pass cannot mask package regression",
			minimum:      "1.00",
			aggregateMin: 0,
			command:      fakeGoCoverageCommand,
			wantFailure:  "package coverage regression: package=" + modulePath + "/pkg/config lane=unit expected-minimum=1.00%",
		},
		{
			name:         "package pass cannot mask aggregate regression",
			minimum:      "100.00",
			aggregateMin: 100.1,
			command:      fakeGoCoverageCommandPassing,
			wantFailure:  "go coverage 100.0% is below minimum 100.1%",
			rejectText:   "package coverage regression",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := writePackageMinimumManifest(t, "unit", modulePath+"/pkg/config", tc.minimum)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			commandRunner = tc.command
			stdoutWriter = &stdout
			stderrWriter = &stderr

			err := execute(config{
				suite:           "unit",
				min:             tc.aggregateMin,
				coverpkg:        modulePath + "/pkg/config",
				packages:        "./pkg/config",
				packageManifest: manifestPath,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantFailure) {
				t.Fatalf("execute() error = %v, want failure containing %q", err, tc.wantFailure)
			}
			if tc.rejectText != "" && strings.Contains(err.Error(), tc.rejectText) {
				t.Fatalf("execute() error = %q, did not expect %q", err.Error(), tc.rejectText)
			}
			if stderr.Len() != 0 {
				t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
			}
		})
	}
}

func writePackageMinimumManifest(t *testing.T, lane string, importPath string, minimum string) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), lane+"-minimums.json")
	data := fmt.Sprintf("{\n  \"version\": 1,\n  \"lane\": %q,\n  \"packages\": [\n    {\n      \"package\": %q,\n      \"minimum\": %s\n    }\n  ]\n}\n", lane, importPath, minimum)
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write package minimum manifest: %v", err)
	}
	return manifestPath
}

func TestExecuteFailsWhenCoverageBelowMinimumAndZeroCoveragePackage(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 100.1,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := strings.Join([]string{
		"go coverage 100.0% is below minimum 100.1%",
		"go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)",
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
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)"
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
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithTempProfileReport
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

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty stderr", stderr.String())
	}

	markerPath := filepath.Join(os.TempDir(), tempProfileMarkerFilename)
	t.Cleanup(func() {
		_ = os.Remove(markerPath)
	})
	profileData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read temp profile marker: %v", err)
	}
	profilePath := strings.TrimSpace(string(profileData))
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("temp profile %q still exists after run(), stat err = %v", profilePath, err)
	}
	if !strings.Contains(stdout.String(), "total: (statements) 100.0%") {
		t.Fatalf("run() stdout = %q, want total coverage line", stdout.String())
	}
}

func TestRunWrapsCoverageLaneFailure(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandTestFailsWithoutDetail
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
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("run() error = %q, want prefix %q", err.Error(), want)
	}
	if !strings.Contains(err.Error(), "raw failure output from go test") {
		t.Fatalf("run() error = %q, want raw go test output detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureUsingStderrDetail(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithStderr

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
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
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithStdout

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
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
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithoutDetail

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	want := "list go packages: exit status 9"
	if err.Error() != want {
		t.Fatalf("listGoPackages() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenDefaultCoverageDiscoveryMatchesNoBackendPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandWithExcludedPackagesOnly

	_, _, err := resolveCoverageLane(config{})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenFunctionalDiscoveryMatchesNoMaintainedPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandWithCoverageButNoTestPackages

	_, _, err := resolveCoverageLane(config{suite: "functional"})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestListGoPackagesFiltersDuplicatesAndExcludedPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		commandRunner = originalCommandRunner
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

	commandRunner = fakeGoListCommandWithDuplicatesAndExcludedPackages

	packages, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err != nil {
		t.Fatalf("listGoPackages() error = %v", err)
	}

	want := []string{
		modulePath + "/pkg/config",
	}
	if !slices.Equal(packages, want) {
		t.Fatalf("listGoPackages() = %v, want %v", packages, want)
	}

	testPackages, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, false)
	if err != nil {
		t.Fatalf("listGoPackages() for test lane error = %v", err)
	}
	if slices.Contains(testPackages, modulePath+"/pkg/config/exhaustiontests") {
		t.Fatalf("listGoPackages() for test lane = %v, maintenance package must be excluded", testPackages)
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

func tempDirOutsideRepo(t *testing.T) string {
	t.Helper()

	repoRoot := testutil.CanonicalPath(testutil.MustRepoRoot(t))
	candidates := []string{os.TempDir()}
	if runtime.GOOS != "windows" {
		candidates = append(candidates, "/tmp")
	}

	for _, base := range candidates {
		tempRoot, err := os.MkdirTemp(base, "gocoveragecheck-*")
		if err != nil {
			continue
		}
		canonicalTemp := testutil.CanonicalPath(tempRoot)
		if isPathWithin(canonicalTemp, repoRoot) {
			if removeErr := os.RemoveAll(tempRoot); removeErr != nil {
				t.Fatalf("remove in-repo temp dir %q: %v", tempRoot, removeErr)
			}
			continue
		}
		t.Cleanup(func() {
			if removeErr := os.RemoveAll(tempRoot); removeErr != nil {
				t.Fatalf("remove temp dir %q: %v", tempRoot, removeErr)
			}
		})
		return tempRoot
	}

	t.Fatal("could not create temp dir outside repository")
	return ""
}

func isPathWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

	workingDir := filepath.Join(tempDirOutsideRepo(t), "pkg", "service")
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

func TestFindZeroCoveragePackagesFromSummaries(t *testing.T) {
	t.Parallel()

	zeroCoveragePackages := findZeroCoveragePackagesFromSummaries(
		[]packageCoverageSummary{
			{importPath: modulePath + "/pkg/config", coverage: 0},
			{importPath: modulePath + "/pkg/service", coverage: 100},
		},
		map[string]struct{}{
			modulePath + "/pkg/config": {},
		},
	)
	if len(zeroCoveragePackages) != 0 {
		t.Fatalf("findZeroCoveragePackagesFromSummaries() = %v, want none", zeroCoveragePackages)
	}
}
