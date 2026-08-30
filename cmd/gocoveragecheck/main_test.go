package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var emptyCoverageBaseline = map[string]struct{}{}

const emptyPackageCoverageBaselineRelPath = "cmd/gocoveragecheck/testdata/empty-package-baseline.txt"

func emptyPackageCoverageBaselinePath(t *testing.T) string {
	t.Helper()
	return emptyPackageCoverageBaselineRelPath
}

func TestIsBackendCoveragePackage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{name: "factory command", importPath: modulePath + "/cmd/factory", want: false},
		{name: "backend package", importPath: modulePath + "/pkg/config", want: true},
		{name: "contract package", importPath: modulePath + "/pkg/transports/http/contracttests", want: false},
		{name: "integration package", importPath: modulePath + "/pkg/transports/http/servertests/factorysessionsse", want: false},
		{name: "maintenance package", importPath: modulePath + "/pkg/services/factory_runtime/internal/exhaustiontests", want: false},
		{name: "generated api package", importPath: modulePath + "/pkg/transports/http/generated", want: false},
		{name: "generated client package", importPath: modulePath + "/pkg/transports/http/client", want: false},
		{name: "generated mcp package", importPath: modulePath + "/pkg/transports/mcp/generated", want: false},
		{name: "test helper package", importPath: modulePath + "/internal/testutil/runtimefixtures", want: false},
		{name: "functional test package", importPath: modulePath + "/tests/functional/runtime_api", want: false},
		{name: "ui package", importPath: modulePath + "/ui", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isBackendCoveragePackage(tc.importPath); got != tc.want {
				t.Fatalf("isBackendCoveragePackage(%q) = %t, want %t", tc.importPath, got, tc.want)
			}
		})
	}
}

func TestIsFunctionalTestPackage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{name: "backend package", importPath: modulePath + "/pkg/config", want: false},
		{name: "functional runtime package", importPath: modulePath + "/tests/functional/runtime_api", want: true},
		{name: "functional internal helper", importPath: modulePath + "/tests/functional/internal/support", want: false},
		{name: "ui package", importPath: modulePath + "/ui", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isFunctionalTestPackage(tc.importPath); got != tc.want {
				t.Fatalf("isFunctionalTestPackage(%q) = %t, want %t", tc.importPath, got, tc.want)
			}
		})
	}
}

func TestCoverageTestJobs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		cfg         config
		targetOS    string
		logicalCPUs int
		want        int
	}{
		{name: "unit uses the non-Windows runner CPU count", cfg: config{suite: "unit"}, targetOS: "linux", logicalCPUs: 4, want: 4},
		{name: "unit follows another positive non-Windows CPU count", cfg: config{suite: "unit"}, targetOS: "linux", logicalCPUs: 8, want: 8},
		{name: "unit defaults to one Windows coverage builder", cfg: config{suite: "unit"}, targetOS: "windows", logicalCPUs: 64, want: 1},
		{name: "empty suite defaults to one Windows unit builder", cfg: config{}, targetOS: "windows", logicalCPUs: 64, want: 1},
		{name: "functional keeps the shared Windows default", cfg: config{suite: "functional"}, targetOS: "windows", logicalCPUs: 64, want: defaultCoverageJobs},
		{name: "functional keeps the shared non-Windows default", cfg: config{suite: "functional"}, targetOS: "linux", logicalCPUs: 64, want: defaultCoverageJobs},
		{name: "invalid CPU input uses the shared fallback", cfg: config{suite: "unit"}, targetOS: "linux", logicalCPUs: 0, want: defaultCoverageJobs},
		{name: "explicit override wins on Windows", cfg: config{suite: "unit", jobs: 9}, targetOS: "windows", logicalCPUs: 64, want: 9},
		{name: "explicit override wins on non-Windows", cfg: config{suite: "unit", jobs: 7}, targetOS: "linux", logicalCPUs: 4, want: 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cfg.testJobs(tc.targetOS, tc.logicalCPUs); got != tc.want {
				t.Fatalf("config.testJobs(%q, %d) = %d, want %d", tc.targetOS, tc.logicalCPUs, got, tc.want)
			}
		})
	}
}

func TestRunForOSWithCPUUsesDerivedUnitJobs(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var invocations []commandInvocation
	var stdout strings.Builder
	var stderr strings.Builder
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		return fakeGoCoverageCommandPassing(invocation)
	}
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := runForOSWithCPU(config{
		min:       0,
		suite:     "unit",
		totalOnly: true,
		coverpkg:  strings.Join([]string{modulePath + "/pkg/config", modulePath + "/pkg/service"}, ","),
		packages:  "./pkg/config",
		profile:   filepath.Join(t.TempDir(), "coverage.out"),
	}, "linux", 4)
	if err != nil {
		t.Fatalf("runForOSWithCPU() error = %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("coverage invocations = %d, want one", len(invocations))
	}
	if !slices.Contains(invocations[0].args, "-p=4") {
		t.Fatalf("go test args = %v, want CPU-derived -p=4", invocations[0].args)
	}
	if stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf("coverage output = stdout %q, stderr %q; want stdout only", stdout.String(), stderr.String())
	}
}

func TestRunForOSWithCPUUsesExplicitFunctionalJobs(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var invocations []commandInvocation
	var stdout strings.Builder
	var stderr strings.Builder
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		return fakeGoCoverageCommandPassing(invocation)
	}
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := runForOSWithCPU(config{
		min:       0,
		suite:     "functional",
		jobs:      12,
		totalOnly: true,
		coverpkg:  strings.Join([]string{modulePath + "/pkg/config", modulePath + "/pkg/service"}, ","),
		packages:  "./tests/functional/runtime_api",
		profile:   filepath.Join(t.TempDir(), "coverage.out"),
	}, "linux", 4)
	if err != nil {
		t.Fatalf("runForOSWithCPU() error = %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("coverage invocations = %d, want one", len(invocations))
	}
	if !slices.Contains(invocations[0].args, "-p=12") {
		t.Fatalf("go test args = %v, want explicit functional -p=12", invocations[0].args)
	}
	if slices.Contains(invocations[0].args, "-p=4") {
		t.Fatalf("go test args = %v, unexpectedly used CPU-derived unit width", invocations[0].args)
	}
	if stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf("coverage output = stdout %q, stderr %q; want stdout only", stdout.String(), stderr.String())
	}
}

func TestReadPackageCoverageBaselineSkipsCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "baseline.txt")
	if err := os.WriteFile(baselinePath, []byte(strings.Join([]string{
		"# legacy exceptions",
		"",
		modulePath + "/pkg/config",
		"  " + modulePath + "/pkg/service  ",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	got, err := readPackageCoverageBaseline(baselinePath)
	if err != nil {
		t.Fatalf("readPackageCoverageBaseline() error = %v", err)
	}

	want := map[string]struct{}{
		modulePath + "/pkg/config":  {},
		modulePath + "/pkg/service": {},
	}
	if len(got) != len(want) {
		t.Fatalf("readPackageCoverageBaseline() = %v, want %v", got, want)
	}
	for pkg := range want {
		if _, ok := got[pkg]; !ok {
			t.Fatalf("readPackageCoverageBaseline() missing %q in %v", pkg, got)
		}
	}
}

func TestPackageCoverageBaselinePathDefaultsBySuite(t *testing.T) {
	t.Parallel()

	if got := (config{suite: "unit"}).packageCoverageBaselinePath(); got != defaultPackageCoverageBaselinePath {
		t.Fatalf("unit baseline path = %q, want %q", got, defaultPackageCoverageBaselinePath)
	}
	if got := (config{suite: "functional"}).packageCoverageBaselinePath(); got != defaultFunctionalPackageCoverageBaselinePath {
		t.Fatalf("functional baseline path = %q, want %q", got, defaultFunctionalPackageCoverageBaselinePath)
	}
	const override = "custom-functional-baseline.txt"
	if got := (config{suite: "functional", packageBaseline: override}).packageCoverageBaselinePath(); got != override {
		t.Fatalf("explicit baseline path = %q, want %q", got, override)
	}
}

func TestValidateConfigRejectsConflictingManifestOperations(t *testing.T) {
	t.Parallel()

	err := validateConfig(config{generateManifest: "candidate.json", updateManifest: "minimums.json"})
	if err == nil || !strings.Contains(err.Error(), "choose only one") {
		t.Fatalf("validateConfig() error = %v, want conflicting operation diagnostic", err)
	}
	if err := validateConfig(config{updateManifest: "minimums.json"}); err == nil || !strings.Contains(err.Error(), "-update-manifest requires -update-profiles") {
		t.Fatalf("validateConfig() single update error = %v, want sampled-update requirement", err)
	}
	if err := validateConfig(config{updateProfiles: "one.out,two.out"}); err == nil || !strings.Contains(err.Error(), "-update-profiles requires -update-manifest") {
		t.Fatalf("validateConfig() profile-only update error = %v, want manifest requirement", err)
	}
}

func TestValidateConfigRejectsInvalidPackageFloorEpsilon(t *testing.T) {
	t.Parallel()

	for _, epsilon := range []float64{-0.01, -1} {
		err := validateConfig(config{packageFloorEpsilon: epsilon})
		if err == nil || !strings.Contains(err.Error(), "-package-floor-epsilon must be a finite non-negative percentage-point value") {
			t.Fatalf("validateConfig(%v) error = %v, want actionable epsilon diagnostic", epsilon, err)
		}
	}
}

func TestFindInsufficientCoveragePackagesSkipsBaselinedPackages(t *testing.T) {
	t.Parallel()

	summaries := []packageCoverageSummary{
		{importPath: modulePath + "/pkg/config", coverage: 74.4},
		{importPath: modulePath + "/pkg/service", coverage: 82.1},
		{importPath: modulePath + "/pkg/uncovered", coverage: 0},
	}

	got := findInsufficientCoveragePackages(summaries, 80, map[string]struct{}{
		modulePath + "/pkg/config": {},
	})

	want := []packageCoverageSummary{
		{importPath: modulePath + "/pkg/uncovered", coverage: 0},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("findInsufficientCoveragePackages() = %v, want %v", got, want)
	}
}

func TestSummarizePackageCoverageFromTotalsUsesOnlySelectedSuiteProfile(t *testing.T) {
	t.Parallel()

	coveredPackage := modulePath + "/pkg/services/automation/service"
	unitCoveredButFunctionallyUntouched := modulePath + "/pkg/workers/worktree"
	got := summarizePackageCoverageFromTotals(
		map[string]packageCoverageTotals{
			coveredPackage: {coveredStatements: 8, totalStatements: 10},
			unitCoveredButFunctionallyUntouched: {
				coveredStatements: 0,
				totalStatements:   10,
			},
		},
		[]string{unitCoveredButFunctionallyUntouched, coveredPackage},
	)

	want := []packageCoverageSummary{
		{importPath: coveredPackage, coverage: 80},
		{importPath: unitCoveredButFunctionallyUntouched, coverage: 0},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("summarizePackageCoverageFromTotals() = %v, want %v", got, want)
	}
}

func TestEvaluateCoverageFlagsBackendPackagesMissingFromProfile(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 82.5%\n",
		modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/pkg/service\t\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageFlagsBackendPackagesMissingFromProfileWithOKSummary(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 82.5%\n",
		"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements\n"+
			"ok  "+modulePath+"/pkg/service\t(cached)\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageFlagsBackendPackagesMissingFromProfileWithCoverpkgOKSummary(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 82.5%\n",
		"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/config, "+modulePath+"/pkg/service, "+modulePath+"/pkg/transports/http/client\n"+
			"ok  "+modulePath+"/pkg/service\t(cached)\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageFlagsBackendPackagesPresentWithZeroCoverage(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		"",
	}, "\n"))

	result, _, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/pkg/service\t\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}
	if result.actual != 62.5 {
		t.Fatalf("actual coverage = %v, want 62.5", result.actual)
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageSkipsExcludedZeroCoveragePackages(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		modulePath + "/internal/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		modulePath+"/pkg/service\t\tcoverage: 100.0% of statements\n"+
			modulePath+"/pkg/transports/http/client\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/internal/testutil/runtimefixtures\t\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
			modulePath + "/internal/testutil/runtimefixtures",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
}

func TestEvaluateCoverageSkipsExcludedZeroCoveragePackagesWithOKSummary(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		modulePath + "/internal/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		"ok  "+modulePath+"/pkg/service\t0.111s\tcoverage: 100.0% of statements\n"+
			"ok  "+modulePath+"/pkg/transports/http/client\t(cached)\tcoverage: 0.0% of statements\n"+
			"ok  "+modulePath+"/internal/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
			modulePath + "/internal/testutil/runtimefixtures",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
}

func TestEvaluateCoverageSkipsExcludedZeroCoveragePackagesWithCoverpkgOKSummary(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/transports/http/client/client.go:1.1,2.1 4 0",
		modulePath + "/internal/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		"ok  "+modulePath+"/pkg/service\t0.111s\tcoverage: 100.0% of statements\n"+
			"ok  "+modulePath+"/pkg/transports/http/client\t(cached)\tcoverage: 0.0% of statements in "+modulePath+"/pkg/transports/http/client, "+modulePath+"/pkg/service\n"+
			"ok  "+modulePath+"/internal/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements in "+modulePath+"/internal/testutil/runtimefixtures, "+modulePath+"/pkg/service\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
			modulePath + "/internal/testutil/runtimefixtures",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if totalLine != "total: (statements) 100.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 100.0%")
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
}

func TestEvaluateCoverageSupportsRepositoryRelativeProfilePaths(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		"pkg\\config\\config.go:1.1,2.1 2 0",
		"pkg\\service\\factory.go:1.1,2.1 4 1",
		"",
	}, "\n"))

	result, _, err := evaluateCoverage(
		"total: (statements) 80.0%\n",
		modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/pkg/service\t\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}
	if result.actual != 66.66666666666667 {
		t.Fatalf("actual coverage = %v, want 66.66666666666667", result.actual)
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageIgnoresExternalTotalReportAndUsesMergedProfileCoverage(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
		modulePath + "/pkg/config/config.go:3.1,4.1 2 0",
		modulePath + "/pkg/config/config.go:3.1,4.1 2 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"not a total report\n",
		"",
		profilePath,
		repoRoot,
		[]string{modulePath + "/pkg/config"},
		80,
		emptyCoverageBaseline,
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}
	if result.actual != 60 {
		t.Fatalf("actual coverage = %v, want 60", result.actual)
	}
	if totalLine != "total: (statements) 60.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 60.0%")
	}
}

func TestMergeCoverageProfilesCombinesIsolatedTestBinaries(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	firstProfile := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
		modulePath + "/pkg/service/factory.go:3.1,4.1 2 1",
		"",
	}, "\n"))
	secondProfile := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:3.1,4.1 2 0",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
		"",
	}, "\n"))
	outputPath := filepath.Join(t.TempDir(), "merged.out")
	coverPackages := []string{modulePath + "/pkg/config", modulePath + "/pkg/service"}

	if err := mergeCoverageProfiles([]string{firstProfile, secondProfile}, outputPath, repoRoot, coverPackages); err != nil {
		t.Fatalf("mergeCoverageProfiles() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read merged profile: %v", err)
	}
	want := strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
		modulePath + "/pkg/service/factory.go:3.1,4.1 2 1",
		"",
	}, "\n")
	if string(got) != want {
		t.Fatalf("merged profile = %q, want %q", got, want)
	}
}

func TestEvaluateCoverageFailsWhenCoverageProfileCannotBeRead(t *testing.T) {
	t.Parallel()

	missingProfilePath := filepath.Join(t.TempDir(), "missing.out")

	_, _, err := evaluateCoverage(
		"total: (statements) 82.5%\n",
		"",
		missingProfilePath,
		filepath.Clean(t.TempDir()),
		[]string{modulePath + "/pkg/config"},
		80,
		emptyCoverageBaseline,
	)
	if err == nil {
		t.Fatal("evaluateCoverage() unexpectedly succeeded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evaluateCoverage() error = %v, want os.ErrNotExist", err)
	}
	wantErrPrefix := "read go coverage profile:"
	if !strings.HasPrefix(err.Error(), wantErrPrefix) {
		t.Fatalf("evaluateCoverage() error = %q, want prefix %q", err.Error(), wantErrPrefix)
	}
	if !strings.Contains(err.Error(), missingProfilePath) {
		t.Fatalf("evaluateCoverage() error = %q, want missing path %q", err.Error(), missingProfilePath)
	}
}
