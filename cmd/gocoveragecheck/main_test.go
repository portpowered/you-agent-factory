package main

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
		modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", result.actual)
	}
	if totalLine != "total: (statements) 82.5%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 82.5%")
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
		"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", result.actual)
	}
	if totalLine != "total: (statements) 82.5%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 82.5%")
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
		"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/config, "+modulePath+"/pkg/service, "+modulePath+"/pkg/generatedclient\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", result.actual)
	}
	if totalLine != "total: (statements) 82.5%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 82.5%")
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
		"",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
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
		modulePath+"/pkg/generatedclient\t\tcoverage: 0.0% of statements\n"+
			modulePath+"/pkg/testutil/runtimefixtures\t\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 81.0 {
		t.Fatalf("actual coverage = %v, want 81.0", result.actual)
	}
	if totalLine != "total: (statements) 81.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 81.0%")
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
		"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements\n"+
			"ok  "+modulePath+"/pkg/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 81.0 {
		t.Fatalf("actual coverage = %v, want 81.0", result.actual)
	}
	if totalLine != "total: (statements) 81.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 81.0%")
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
		"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements in "+modulePath+"/pkg/generatedclient, "+modulePath+"/pkg/service\n"+
			"ok  "+modulePath+"/pkg/testutil/runtimefixtures\t0.321s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/testutil/runtimefixtures, "+modulePath+"/pkg/service\n",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
			modulePath + "/pkg/testutil/runtimefixtures",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	if result.actual != 81.0 {
		t.Fatalf("actual coverage = %v, want 81.0", result.actual)
	}
	if totalLine != "total: (statements) 81.0%" {
		t.Fatalf("total line = %q, want %q", totalLine, "total: (statements) 81.0%")
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
		"",
		profilePath,
		repoRoot,
		[]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		},
	)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}

	wantZeroCoverage := []string{modulePath + "/pkg/config"}
	if !slices.Equal(result.zeroCoveragePackages, wantZeroCoverage) {
		t.Fatalf("zero coverage packages = %v, want %v", result.zeroCoveragePackages, wantZeroCoverage)
	}
}

func TestEvaluateCoverageFailsWhenTotalCoverageCannotBeParsed(t *testing.T) {
	t.Parallel()

	profilePath := writeCoverageProfile(t, "mode: count\n")

	_, _, err := evaluateCoverage(
		"not a total report\n",
		"",
		profilePath,
		filepath.Clean(t.TempDir()),
		[]string{modulePath + "/pkg/config"},
	)
	if err == nil {
		t.Fatal("evaluateCoverage() unexpectedly succeeded")
	}
	if err.Error() != "parse go coverage total: missing total statements line" {
		t.Fatalf("evaluateCoverage() error = %q, want parse total failure", err.Error())
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
	)
	if err == nil {
		t.Fatal("evaluateCoverage() unexpectedly succeeded")
	}
	wantErr := fmt.Sprintf("read go coverage profile: open %s: The system cannot find the file specified.", missingProfilePath)
	if err.Error() != wantErr {
		t.Fatalf("evaluateCoverage() error = %q, want %q", err.Error(), wantErr)
	}
}

func TestParseTotalCoverageFailsWhenTotalLineMissing(t *testing.T) {
	t.Parallel()

	_, _, err := parseTotalCoverage(modulePath + "/pkg/config/config.go:1.1,2.1\t75.0%\n")
	if err == nil {
		t.Fatal("parseTotalCoverage() unexpectedly succeeded")
	}
	if err.Error() != "parse go coverage total: missing total statements line" {
		t.Fatalf("parseTotalCoverage() error = %q, want missing total line error", err.Error())
	}
}

func TestParseTotalCoverageSynthesizesNormalizedTotalLine(t *testing.T) {
	t.Parallel()

	report := modulePath + "/pkg/config/config.go:1.1,2.1\t75.0%\nsummary total: (statements) 82.5%\n"
	actual, totalLine, err := parseTotalCoverage(report)
	if err != nil {
		t.Fatalf("parseTotalCoverage() error = %v", err)
	}

	if actual != 82.5 {
		t.Fatalf("actual coverage = %v, want 82.5", actual)
	}
	if totalLine != "total: (statements) 82.5%" {
		t.Fatalf("total line = %q, want normalized fallback line", totalLine)
	}
}

func TestSplitList(t *testing.T) {
	t.Parallel()

	if got := splitList("alpha, ,gamma", ",", false); !slices.Equal(got, []string{"alpha", "", "gamma"}) {
		t.Fatalf("splitList() with filterEmpty=false = %v, want preserved empty entry", got)
	}
	if got := splitList("alpha  beta   gamma", " ", true); !slices.Equal(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("splitList() with filterEmpty=true = %v, want trimmed non-empty entries", got)
	}
}

func TestParseZeroCoveragePackagesFromReport(t *testing.T) {
	t.Parallel()

	report := strings.Join([]string{
		"",
		"ok  " + modulePath + "/pkg/config\t0.123s\tcoverage: 0.0% of statements",
		modulePath + "/pkg/service\t\tcoverage: 82.5% of statements",
		"total: (statements) 82.5%",
		"not a package coverage line",
		"",
	}, "\n")

	got, err := parseZeroCoveragePackagesFromReport(report)
	if err != nil {
		t.Fatalf("parseZeroCoveragePackagesFromReport() error = %v", err)
	}

	want := map[string]bool{
		modulePath + "/pkg/config": true,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("parseZeroCoveragePackagesFromReport() = %v, want %v", got, want)
	}
}

func TestParseCoverageProfileRejectsMalformedInputs(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	cases := []struct {
		name        string
		profileData string
		wantErr     string
	}{
		{
			name:        "empty profile",
			profileData: "",
			wantErr:     "parse go coverage profile: empty profile",
		},
		{
			name:        "missing mode header",
			profileData: "pkg/config/config.go:1.1,2.1 2 1\n",
			wantErr:     "parse go coverage profile: missing mode header",
		},
		{
			name:        "malformed line shape",
			profileData: "mode: count\npkg/config/config.go:1.1,2.1 2\n",
			wantErr:     "parse go coverage profile: malformed line 2",
		},
		{
			name:        "malformed file range",
			profileData: "mode: count\npkg/config/config.go 2 1\n",
			wantErr:     "parse go coverage profile: malformed file range on line 2",
		},
		{
			name:        "invalid statement count",
			profileData: "mode: count\npkg/config/config.go:1.1,2.1 nope 1\n",
			wantErr:     "parse go coverage profile statements on line 2: strconv.Atoi: parsing \"nope\": invalid syntax",
		},
		{
			name:        "invalid execution count",
			profileData: "mode: count\npkg/config/config.go:1.1,2.1 2 nope\n",
			wantErr:     "parse go coverage profile execution count on line 2: strconv.Atoi: parsing \"nope\": invalid syntax",
		},
		{
			name:        "import path escapes repository root",
			profileData: "mode: count\n../outside/pkg/config.go:1.1,2.1 2 1\n",
			wantErr:     "parse go coverage profile import path on line 2: profile path \"../outside/pkg/config.go\" escapes repository root",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseCoverageProfile([]byte(tc.profileData), repoRoot)
			if err == nil {
				t.Fatal("parseCoverageProfile() unexpectedly succeeded")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("parseCoverageProfile() error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCoverageImportPathRejectsMalformedPaths(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	outsidePath := filepath.Join(repoRoot, "..", "outside", "pkg", "config.go")
	cases := []struct {
		name     string
		filePath string
		wantErr  string
	}{
		{
			name:     "empty path",
			filePath: " \t ",
			wantErr:  "empty file path",
		},
		{
			name:     "repository escape",
			filePath: outsidePath,
			wantErr:  fmt.Sprintf("profile path %q escapes repository root", outsidePath),
		},
		{
			name:     "module qualified without package directory",
			filePath: modulePath,
			wantErr:  fmt.Sprintf("profile path %q does not include a package directory", modulePath),
		},
		{
			name:     "relative path without package directory",
			filePath: "config.go",
			wantErr:  "profile path \"config.go\" does not include a package directory",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := coverageImportPath(tc.filePath, repoRoot)
			if err == nil {
				t.Fatal("coverageImportPath() unexpectedly succeeded")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("coverageImportPath() error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCoverageImportPathNormalizesSupportedPaths(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	absolutePath := filepath.Join(repoRoot, "pkg", "config", "config.go")
	cases := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "module qualified path",
			filePath: modulePath + "/pkg/config/config.go",
			want:     modulePath + "/pkg/config",
		},
		{
			name:     "relative path with dot prefix",
			filePath: "./pkg/config/config.go",
			want:     modulePath + "/pkg/config",
		},
		{
			name:     "absolute path inside repository root",
			filePath: absolutePath,
			want:     modulePath + "/pkg/config",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := coverageImportPath(tc.filePath, repoRoot)
			if err != nil {
				t.Fatalf("coverageImportPath() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("coverageImportPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatZeroCoverageFailure(t *testing.T) {
	t.Parallel()

	got := formatZeroCoverageFailure([]string{
		modulePath + "/pkg/config",
		modulePath + "/pkg/service",
	})
	want := "go coverage found backend-owned packages with 0% statement coverage: " +
		modulePath + "/pkg/config, " + modulePath + "/pkg/service"
	if got != want {
		t.Fatalf("formatZeroCoverageFailure() = %q, want %q", got, want)
	}
}

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
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoCoverageCommandCoverFailsWithStderr

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
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoCoverageCommandCoverFailsWithStdout

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
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoCoverageCommandTestFailsWithoutDetail

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
	defer func() {
		execCommand = originalExecCommand
	}()

	execCommand = fakeGoCoverageCommandCoverFailsWithoutDetail

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
	if got != repoRoot {
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

func TestMainFailsWhenCoverageBelowMinimumViaFailf(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=90",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); strings.Contains(got, "meets minimum") {
		t.Fatalf("main() stdout = %q, did not expect success message", got)
	}
	wantFailure := "go coverage 82.5% is below minimum 90.0%\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainReportsPassingCoverageWithoutFailing(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 0 {
		t.Fatalf("main() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "Go coverage 82.5% meets minimum 80.0%.") {
		t.Fatalf("main() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("main() stderr = %q, want empty stderr", got)
	}
}

func TestMainFailsWhenZeroCoveragePackagesDetectedViaFailf(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoveragePackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandWithOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageCoverpkgOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandWithCoverpkgOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestGoCoverageCheckFakeGoProcess(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config\t\tcoverage: 0.0% of statements\n"+
				modulePath+"/pkg/generatedclient\t\tcoverage: 0.0% of statements\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func writeCoverageProfile(t *testing.T, contents string) string {
	t.Helper()

	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}
	return profilePath
}

func fakeGoCoverageCommand(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcess", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandPassing(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessPassing", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandWithOKSummary(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessWithOKSummary", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandWithCoverpkgOKSummary(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessWithCoverpkgOKSummary", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func TestGoCoverageCheckFakeGoProcessWithOKSummary(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout,
			"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements\n"+
				"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessPassing(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config/config.go:1.1,2.1\t75.0%\n"+
				modulePath+"/pkg/service/factory.go:1.1,2.1\t100.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessWithCoverpkgOKSummary(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout,
			"ok  "+modulePath+"/pkg/config\t0.123s\tcoverage: 0.0% of statements in "+modulePath+"/pkg/config, "+modulePath+"/pkg/service, "+modulePath+"/pkg/generatedclient\n"+
				"ok  "+modulePath+"/pkg/generatedclient\t(cached)\tcoverage: 0.0% of statements in "+modulePath+"/pkg/generatedclient, "+modulePath+"/pkg/service\n",
		)
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/service/factory.go:1.1,2.1\t80.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessWithTempProfileReport(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stdout, "TEMP_PROFILE=%s\n", profilePath)
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout,
			modulePath+"/pkg/config/config.go:1.1,2.1\t75.0%\n"+
				modulePath+"/pkg/service/factory.go:1.1,2.1\t100.0%\n"+
				"total: (statements) 82.5%\n",
		)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithStderr(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stderr, "stderr detail from cover tool")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithStdout(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		fmt.Fprint(os.Stdout, "stdout detail from cover tool")
		os.Exit(4)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoCoverageCheckFakeGoProcessTestFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	if len(args) >= 2 && args[1] == "test" {
		os.Exit(7)
	}

	fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
	os.Exit(2)
}

func TestGoCoverageCheckFakeGoProcessCoverFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) == 0 || args[0] != "go" {
		return
	}

	switch {
	case len(args) >= 2 && args[1] == "test":
		profilePath := ""
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-coverprofile=") {
				profilePath = strings.TrimPrefix(arg, "-coverprofile=")
				break
			}
		}
		if profilePath == "" {
			fmt.Fprint(os.Stderr, "missing coverprofile argument")
			os.Exit(2)
		}
		profile := strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n")
		if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fake profile: %v", err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, modulePath+"/pkg/config\t\tcoverage: 75.0% of statements\n")
		os.Exit(0)
	case len(args) == 5 && args[1] == "tool" && args[2] == "cover" && args[3] == "-func":
		os.Exit(8)
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake go args: %v", args)
		os.Exit(2)
	}
}

func TestGoListCommandFailsWithStderr(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprint(os.Stderr, "stderr detail from go list")
	os.Exit(5)
}

func TestGoListCommandFailsWithStdout(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprint(os.Stdout, "stdout detail from go list")
	os.Exit(6)
}

func TestGoListCommandFailsWithoutDetail(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	os.Exit(9)
}

func TestGoListCommandWithExcludedPackagesOnly(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/generatedclient")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/testutil/runtimefixtures")
	os.Exit(0)
}

func TestGoListCommandWithCoverageButNoTestPackages(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	if slices.Contains(args[2:], "./tests/functional/...") {
		fmt.Fprintln(os.Stdout, modulePath+"/tests/functional/internal/support")
		os.Exit(0)
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config")
	os.Exit(0)
}

func TestGoListCommandWithDuplicatesAndExcludedPackages(t *testing.T) {
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 2 || args[0] != "go" || args[1] != "list" {
		return
	}

	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/config")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/generatedclient")
	fmt.Fprintln(os.Stdout, modulePath+"/pkg/testutil/runtimefixtures")
	fmt.Fprintln(os.Stdout, modulePath+"/cmd/factory")
	os.Exit(0)
}

func helperCommandArgs(argv []string) ([]string, bool) {
	for index, arg := range argv {
		if arg == "--" {
			return argv[index+1:], true
		}
	}
	return nil, false
}

func parseTempProfilePath(t *testing.T, output string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "TEMP_PROFILE=") {
			return strings.TrimPrefix(line, "TEMP_PROFILE=")
		}
	}
	t.Fatalf("TEMP_PROFILE marker missing from output %q", output)
	return ""
}

func fakeGoCoverageCommandWithTempProfileReport(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessWithTempProfileReport", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandCoverFailsWithStderr(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessCoverFailsWithStderr", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandCoverFailsWithStdout(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessCoverFailsWithStdout", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandTestFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessTestFailsWithoutDetail", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoCoverageCommandCoverFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoCoverageCheckFakeGoProcessCoverFailsWithoutDetail", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandFailsWithStderr(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandFailsWithStderr", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandFailsWithStdout(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandFailsWithStdout", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandFailsWithoutDetail(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandFailsWithoutDetail", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandWithExcludedPackagesOnly(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandWithExcludedPackagesOnly", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandWithCoverageButNoTestPackages(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandWithCoverageButNoTestPackages", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}

func fakeGoListCommandWithDuplicatesAndExcludedPackages(name string, args ...string) *exec.Cmd {
	testBinary, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("resolve test binary: %v", err))
	}

	cmdArgs := append([]string{"-test.run=TestGoListCommandWithDuplicatesAndExcludedPackages", "--", name}, args...)
	return exec.Command(testBinary, cmdArgs...)
}
