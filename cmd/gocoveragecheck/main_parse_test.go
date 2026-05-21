package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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

func TestParseTotalCoverageRejectsMalformedPercentageToken(t *testing.T) {
	t.Parallel()

	_, _, err := parseTotalCoverage("total: (statements) 1.2.3%\n")
	if err == nil {
		t.Fatal("parseTotalCoverage() unexpectedly succeeded")
	}

	wantErr := "parse go coverage percentage \"1.2.3\": strconv.ParseFloat: parsing \"1.2.3\": invalid syntax"
	if err.Error() != wantErr {
		t.Fatalf("parseTotalCoverage() error = %q, want %q", err.Error(), wantErr)
	}
}

func TestParseZeroCoveragePackagesFromReportRejectsMalformedPercentageToken(t *testing.T) {
	t.Parallel()

	report := modulePath + "/pkg/config\t\tcoverage: 1.2.3% of statements\n"
	_, err := parseZeroCoveragePackagesFromReport(report)
	if err == nil {
		t.Fatal("parseZeroCoveragePackagesFromReport() unexpectedly succeeded")
	}

	wantErr := "parse go coverage package percentage \"1.2.3\": strconv.ParseFloat: parsing \"1.2.3\": invalid syntax"
	if err.Error() != wantErr {
		t.Fatalf("parseZeroCoveragePackagesFromReport() error = %q, want %q", err.Error(), wantErr)
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
