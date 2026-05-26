package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPathsDocsOnly(t *testing.T) {
	t.Parallel()

	result := classifyPaths([]string{
		"docs/reference/authoring-factories.md",
		"README.md",
	})

	if result.Classification != classificationDocsOnly {
		t.Fatalf("classification = %q, want %q", result.Classification, classificationDocsOnly)
	}
	if got := strings.Join(result.Areas, ","); got != "docs" {
		t.Fatalf("areas = %q, want docs", got)
	}
}

func TestClassifyPathsUIOnlyAllowsDocumentationCompanionChanges(t *testing.T) {
	t.Parallel()

	result := classifyPaths([]string{
		"ui/src/App.tsx",
		"ui/package.json",
		"docs/internal/development/dashboard-ui-workflow-baseline.md",
	})

	if result.Classification != classificationUIOnly {
		t.Fatalf("classification = %q, want %q", result.Classification, classificationUIOnly)
	}
	if got := strings.Join(result.Areas, ","); got != "docs,ui" {
		t.Fatalf("areas = %q, want docs,ui", got)
	}
}

func TestClassifyPathsBackendOnlyAllowsDocumentationCompanionChanges(t *testing.T) {
	t.Parallel()

	result := classifyPaths([]string{
		"pkg/service/server.go",
		"cmd/factory/main.go",
		"tests/functional/runtime_api/server_test.go",
		"docs/architecture/architecture.md",
	})

	if result.Classification != classificationBackendOnly {
		t.Fatalf("classification = %q, want %q", result.Classification, classificationBackendOnly)
	}
	if got := strings.Join(result.Areas, ","); got != "backend,docs" {
		t.Fatalf("areas = %q, want backend,docs", got)
	}
}

func TestClassifyPathsMarksSharedRiskForCrossCuttingAndUnknownSurfaces(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		paths []string
	}{
		{
			name: "api-contract",
			paths: []string{
				"api/openapi-main.yaml",
			},
		},
		{
			name: "mixed-ui-and-backend",
			paths: []string{
				"ui/src/App.tsx",
				"pkg/service/server.go",
			},
		},
		{
			name: "workflow",
			paths: []string{
				".github/workflows/ci.yml",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := classifyPaths(tc.paths)
			if result.Classification != classificationSharedRisk {
				t.Fatalf("classification = %q, want %q", result.Classification, classificationSharedRisk)
			}
			if !strings.Contains(result.Reason, "full verification safety path") {
				t.Fatalf("reason = %q, want full-run explanation", result.Reason)
			}
		})
	}
}

func TestRunWritesGitHubOutputsAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	changedFilesPath := filepath.Join(tempDir, "changed-files.txt")
	if err := os.WriteFile(changedFilesPath, []byte(strings.Join([]string{
		"ui/src/App.tsx",
		"docs/reference/authoring-factories.md",
		"ui/src/App.tsx",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write changed files fixture: %v", err)
	}

	outputPath := filepath.Join(tempDir, "github-output.txt")
	summaryPath := filepath.Join(tempDir, "github-summary.md")

	t.Setenv("GITHUB_OUTPUT", outputPath)
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{changedFilesPath: changedFilesPath}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	if got := stdout.String(); !strings.Contains(got, "classification=ui-only") {
		t.Fatalf("stdout = %q, want ui-only summary", got)
	}

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read GitHub output: %v", err)
	}
	output := string(outputBytes)
	if !strings.Contains(output, "classification=ui-only") {
		t.Fatalf("GitHub output = %q, want classification", output)
	}
	if !strings.Contains(output, "areas=docs,ui") {
		t.Fatalf("GitHub output = %q, want areas", output)
	}
	if !strings.Contains(output, "changed_files_count=2") {
		t.Fatalf("GitHub output = %q, want deduplicated changed file count", output)
	}

	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read GitHub summary: %v", err)
	}
	summary := string(summaryBytes)
	if !strings.Contains(summary, "## Pull request impact classification") {
		t.Fatalf("GitHub summary = %q, want heading", summary)
	}
	if !strings.Contains(summary, "- Classification: `ui-only`") {
		t.Fatalf("GitHub summary = %q, want classification bullet", summary)
	}
	if !strings.Contains(summary, "- `ui/src/App.tsx`") {
		t.Fatalf("GitHub summary = %q, want changed file list", summary)
	}
}

func TestResolveChangedPathsValidatesInputs(t *testing.T) {
	t.Parallel()

	_, err := resolveChangedPaths(config{})
	if err == nil || err.Error() != "either -changed-files-path or both -base and -head must be provided" {
		t.Fatalf("resolveChangedPaths() error = %v, want missing-input error", err)
	}

	_, err = resolveChangedPaths(config{baseRef: "origin/main"})
	if err == nil || err.Error() != "both -base and -head must be provided together" {
		t.Fatalf("resolveChangedPaths() error = %v, want paired-ref error", err)
	}
}
