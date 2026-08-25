package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderConsoleSummaryOnlyPrintsPkgCoverageAndPackageLatencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	coveragePath := filepath.Join(root, "coverage.json")
	timingPath := filepath.Join(root, "timing.json")
	if err := os.WriteFile(coveragePath, []byte(`{"packages":[{"package":"example/pkg/alpha","coveragePercent":80},{"package":"example/cmd/tool","coveragePercent":90}]}`), 0o644); err != nil {
		t.Fatalf("write coverage fixture: %v", err)
	}
	if err := os.WriteFile(timingPath, []byte(`{"packages":[{"package":"example/pkg/alpha","seconds":0.125,"outcome":"fail"},{"package":"example/cmd/tool","seconds":0.001,"outcome":"pass"}]}`), 0o644); err != nil {
		t.Fatalf("write timing fixture: %v", err)
	}

	var output bytes.Buffer
	if err := renderConsoleSummary(coveragePath, timingPath, &output); err != nil {
		t.Fatalf("render console summary: %v", err)
	}
	got := output.String()
	for _, expected := range []string{"Unit coverage for pkg/:", "pkg/alpha 80.0%", "Unit package latencies:", "pkg/alpha 0.125s"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("console summary missing %q:\n%s", expected, got)
		}
	}
	for _, unwanted := range []string{"cmd/tool", "pass", "fail"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("console summary contains unwanted %q:\n%s", unwanted, got)
		}
	}
}

func TestValidateConfigRequiresOwnedArtifacts(t *testing.T) {
	t.Parallel()
	if err := validateConfig(config{}); err == nil {
		t.Fatal("validateConfig() error = nil, want missing artifact failure")
	}
}
