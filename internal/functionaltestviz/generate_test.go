package functionaltestviz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestWriteCatalogFileCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outputPath := filepath.Join(root, ".artifacts", "functional-test-viz", "functional-tests.md")
	const body = "# Functional tests\n"
	if err := functionaltestviz.WriteCatalogFile(outputPath, body); err != nil {
		t.Fatalf("WriteCatalogFile: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read written catalog: %v", err)
	}
	if string(got) != body {
		t.Fatalf("written catalog = %q, want %q", got, body)
	}
}

func TestWriteCatalogFileRequiresPath(t *testing.T) {
	t.Parallel()

	err := functionaltestviz.WriteCatalogFile("  ", "# Functional tests\n")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("WriteCatalogFile(\"\") error = %v, want required-path guidance", err)
	}
}

func TestGenerateWritesDefaultArtifactPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	functionalRoot := filepath.Join(root, "tests", "functional")
	writeGenerateFixtures(t, root, functionalRoot)

	coveragePath := filepath.Join(root, "coverage-summary.json")
	if err := os.WriteFile(coveragePath, []byte(generateCoverageSummaryJSON), 0o644); err != nil {
		t.Fatalf("write coverage summary: %v", err)
	}

	if err := functionaltestviz.Generate(functionaltestviz.GenerateConfig{
		RepositoryRoot:      root,
		CoverageSummaryPath: coveragePath,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	outputPath := filepath.Join(root, filepath.FromSlash(functionaltestviz.DefaultOutputPath))
	first, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated catalog: %v", err)
	}
	if !strings.Contains(string(first), "# Functional tests\n") {
		t.Fatalf("generated catalog missing heading:\n%s", first)
	}
	if !strings.Contains(string(first), "TestHelp") {
		t.Fatalf("generated catalog missing inventoried test:\n%s", first)
	}
	if !strings.Contains(string(first), "Golden provenance:") {
		t.Fatalf("generated catalog missing golden provenance:\n%s", first)
	}
	if !strings.Contains(string(first), "## Package coverage\n") {
		t.Fatalf("generated catalog missing package coverage:\n%s", first)
	}

	if err := functionaltestviz.Generate(functionaltestviz.GenerateConfig{
		RepositoryRoot:      root,
		CoverageSummaryPath: coveragePath,
	}); err != nil {
		t.Fatalf("Generate second: %v", err)
	}
	second, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read regenerated catalog: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("consecutive Generate writes diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestGenerateFailsClosedForMissingCoverageSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	functionalRoot := filepath.Join(root, "tests", "functional")
	if err := os.MkdirAll(filepath.Join(functionalRoot, "transport"), 0o755); err != nil {
		t.Fatalf("mkdir functional root: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(functionalRoot, "transport", "help_test.go"),
		[]byte("package transport\n\n// TestHelp verifies help.\nfunc TestHelp(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatalf("write fixture test: %v", err)
	}

	err := functionaltestviz.Generate(functionaltestviz.GenerateConfig{
		RepositoryRoot:      root,
		CoverageSummaryPath: filepath.Join(root, "missing-coverage-summary.json"),
		OutputPath:          filepath.Join(root, "out.md"),
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Generate(missing coverage) error = %v, want not-found guidance", err)
	}
}

func writeGenerateFixtures(t *testing.T, root, functionalRoot string) {
	t.Helper()

	transportDir := filepath.Join(functionalRoot, "transport", "cli")
	workersDir := filepath.Join(functionalRoot, "workers", "inference", "openai")
	runtimeDir := filepath.Join(functionalRoot, "runtime_api")
	harnessDir := filepath.Join(functionalRoot, "internal", "support")
	manifestDir := filepath.Join(root, "testdata", "goldens", "openai", "invoke")
	for _, dir := range []string{transportDir, workersDir, runtimeDir, harnessDir, manifestDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	files := map[string]string{
		filepath.Join(transportDir, "help_test.go"): `package cli

import "testing"

// TestHelp verifies help output.
func TestHelp(t *testing.T) {}
`,
		filepath.Join(workersDir, "invoke_long_test.go"): `//go:build functionallong

package openai

import "testing"

// TestInvoke verifies provider invoke replay.
//
//golden: testdata/goldens/openai/invoke/manifest.json
func TestInvoke(t *testing.T) {}
`,
		filepath.Join(runtimeDir, "session_test.go"): `package runtime_api

import "testing"

func TestSession(t *testing.T) {}
`,
		filepath.Join(harnessDir, "helpers_test.go"): `package support

import "testing"

func TestHelper(t *testing.T) {}
`,
		filepath.Join(manifestDir, "manifest.json"): `{
  "id": "openai-invoke",
  "provider": "openai",
  "case": "invoke",
  "fidelityClass": "partial-stream"
}
`,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
}

const generateCoverageSummaryJSON = `{
  "coveredStatements": 1,
  "measurableStatements": 2,
  "coveragePercent": 50.0,
  "packages": [
    {
      "package": "github.com/portpowered/infinite-you/pkg/example",
      "coveredStatements": 1,
      "measurableStatements": 2,
      "coveragePercent": 50.0,
      "packageFloor": 40.0,
      "measurementException": null
    }
  ]
}
`
