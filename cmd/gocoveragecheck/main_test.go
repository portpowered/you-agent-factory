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
		{name: "factory command", importPath: modulePath + "/cmd/factory", want: true},
		{name: "backend package", importPath: modulePath + "/pkg/config", want: true},
		{name: "generated api package", importPath: modulePath + "/pkg/api/generated", want: false},
		{name: "generated client package", importPath: modulePath + "/pkg/generatedclient", want: false},
		{name: "test helper package", importPath: modulePath + "/pkg/testutil/runtimefixtures", want: false},
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

func TestIsBackendTestPackage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{name: "backend package", importPath: modulePath + "/pkg/config", want: true},
		{name: "functional runtime package", importPath: modulePath + "/tests/functional/runtime_api", want: true},
		{name: "functional internal helper", importPath: modulePath + "/tests/functional/internal/support", want: false},
		{name: "ui package", importPath: modulePath + "/ui", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isBackendTestPackage(tc.importPath); got != tc.want {
				t.Fatalf("isBackendTestPackage(%q) = %t, want %t", tc.importPath, got, tc.want)
			}
		})
	}
}

func TestResolveCoverageLaneDefaults(t *testing.T) {
	coverPackages, testPackages, err := resolveCoverageLane(config{})
	if err != nil {
		t.Fatalf("resolveCoverageLane() error = %v", err)
	}

	if !slices.Contains(coverPackages, modulePath+"/pkg/config") {
		t.Fatalf("cover packages missing backend package: %v", coverPackages)
	}
	if slices.Contains(coverPackages, modulePath+"/pkg/generatedclient") {
		t.Fatalf("cover packages unexpectedly include generated client: %v", coverPackages)
	}
	if slices.Contains(coverPackages, modulePath+"/pkg/testutil") {
		t.Fatalf("cover packages unexpectedly include test helper package: %v", coverPackages)
	}
	if !slices.Contains(testPackages, modulePath+"/tests/functional/runtime_api") {
		t.Fatalf("test packages missing backend functional package: %v", testPackages)
	}
	for _, functionalPackage := range []string{
		modulePath + "/tests/functional/bootstrap_portability",
		modulePath + "/tests/functional/guards_batch",
		modulePath + "/tests/functional/providers",
		modulePath + "/tests/functional/replay_contracts",
		modulePath + "/tests/functional/runtime_api",
		modulePath + "/tests/functional/smoke",
		modulePath + "/tests/functional/workflow",
	} {
		if !slices.Contains(testPackages, functionalPackage) {
			t.Fatalf("test packages missing maintained functional package %q: %v", functionalPackage, testPackages)
		}
	}
	if slices.Contains(testPackages, modulePath+"/tests/functional/internal/support") {
		t.Fatalf("test packages unexpectedly include functional support helpers: %v", testPackages)
	}
}

func TestResolveCoverageLaneOverrides(t *testing.T) {
	t.Parallel()

	cfg := config{
		coverpkg: "example.com/backend, example.com/shared",
		packages: "./pkg/config ./tests/functional/runtime_api",
	}

	coverPackages, testPackages, err := resolveCoverageLane(cfg)
	if err != nil {
		t.Fatalf("resolveCoverageLane() error = %v", err)
	}

	wantCover := []string{"example.com/backend", "example.com/shared"}
	if !slices.Equal(coverPackages, wantCover) {
		t.Fatalf("cover packages = %v, want %v", coverPackages, wantCover)
	}

	wantTests := []string{"./pkg/config", "./tests/functional/runtime_api"}
	if !slices.Equal(testPackages, wantTests) {
		t.Fatalf("test packages = %v, want %v", testPackages, wantTests)
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

func TestEvaluateCoverageFlagsBackendPackagesMissingFromProfile(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
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
			modulePath + "/pkg/generatedclient",
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
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
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
			modulePath + "/pkg/generatedclient",
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
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 82.5%\n",
		"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/config, "+modulePath+"/pkg/service, "+modulePath+"/pkg/generatedclient\n"+
			"ok  "+modulePath+"/pkg/service\t(cached)\tcoverage: 100.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
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
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		modulePath + "/pkg/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		modulePath+"/pkg/service\t\tcoverage: 100.0% of statements\n"+
			modulePath+"/pkg/generatedclient\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/pkg/testutil/runtimefixtures\t\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
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
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		modulePath + "/pkg/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		"ok  "+modulePath+"/pkg/service\t0.111s\tcoverage: 100.0% of statements\n"+
			"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements\n"+
			"ok  "+modulePath+"/pkg/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
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
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		modulePath + "/pkg/testutil/runtimefixtures/factory.go:1.1,2.1 3 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"total: (statements) 81.0%\n",
		"ok  "+modulePath+"/pkg/service\t0.111s\tcoverage: 100.0% of statements\n"+
			"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements in "+modulePath+"/pkg/generatedclient, "+modulePath+"/pkg/service\n"+
			"ok  "+modulePath+"/pkg/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/testutil/runtimefixtures, "+modulePath+"/pkg/service\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
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
