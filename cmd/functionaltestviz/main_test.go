package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresCoverageSummary(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(config{
		repositoryRoot: t.TempDir(),
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "coverage-summary path is required") {
		t.Fatalf("run() error = %v, want required coverage-summary guidance", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunWritesCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	functionalRoot := filepath.Join(root, "tests", "functional", "transport")
	if err := os.MkdirAll(functionalRoot, 0o755); err != nil {
		t.Fatalf("mkdir functional root: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(functionalRoot, "help_test.go"),
		[]byte("package transport\n\nimport \"testing\"\n\n// TestHelp verifies help.\nfunc TestHelp(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatalf("write fixture test: %v", err)
	}

	coveragePath := filepath.Join(root, "coverage-summary.json")
	const coverage = `{
  "coveredStatements": 0,
  "measurableStatements": 0,
  "coveragePercent": 0.0,
  "packages": []
}
`
	if err := os.WriteFile(coveragePath, []byte(coverage), 0o644); err != nil {
		t.Fatalf("write coverage summary: %v", err)
	}

	outputPath := filepath.Join(root, "out", "functional-tests.md")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(config{
		repositoryRoot:      root,
		coverageSummaryPath: coveragePath,
		outputPath:          outputPath,
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok", stdout.String())
	}
	if !strings.Contains(stderr.String(), "wrote catalog to") {
		t.Fatalf("stderr = %q, want wrote-catalog message", stderr.String())
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(got), "TestHelp") {
		t.Fatalf("output missing TestHelp:\n%s", got)
	}
}
