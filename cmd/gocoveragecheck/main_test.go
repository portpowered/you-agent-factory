package main

import (
	"os"
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

func TestEvaluateCoverageFlagsZeroCoverageBackendPackages(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 0",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		modulePath + "/pkg/generatedclient/client.go:1.1,2.1 4 0",
		"",
	}, "\n"))

	result, totalLine, err := evaluateCoverage(
		"github.com/portpowered/infinite-you/pkg/config\t\tcoverage: 0.0% of statements\n"+
			"total: (statements) 82.5%\n",
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

func writeCoverageProfile(t *testing.T, contents string) string {
	t.Helper()

	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}
	return profilePath
}
