package main

import (
	"fmt"
	"path/filepath"
	"slices"
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

func TestCompactCoverageOutputRemovesCoverpkgInventory(t *testing.T) {
	t.Parallel()

	input := "ok  " + modulePath + "/tests/functional/runtime_api\t0.123s\t" +
		"coverage: 33.2% of statements in " + modulePath + "/pkg/config, " +
		modulePath + "/pkg/workers/worktree\nFAIL\n"
	got := compactCoverageOutput(input)
	want := "ok  " + modulePath + "/tests/functional/runtime_api\t0.123s\t" +
		"coverage: 33.2% of statements\nFAIL\n"
	if got != want {
		t.Fatalf("compactCoverageOutput() = %q, want %q", got, want)
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
