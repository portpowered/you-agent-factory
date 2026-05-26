package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
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

func TestClassifyPathsDeletionOnlyChangesStillRouteToOwnedNarrowLanes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		paths              []string
		wantClassification string
		wantAreas          string
	}{
		{
			name:               "docs-only-deletions",
			paths:              []string{"docs/internal/development/development.md", "README.md"},
			wantClassification: classificationDocsOnly,
			wantAreas:          "docs",
		},
		{
			name:               "ui-only-deletions",
			paths:              []string{"docs/internal/development/development.md", "ui/src/App.tsx"},
			wantClassification: classificationUIOnly,
			wantAreas:          "docs,ui",
		},
		{
			name:               "backend-only-deletions",
			paths:              []string{"docs/internal/development/development.md", "pkg/service/server.go"},
			wantClassification: classificationBackendOnly,
			wantAreas:          "backend,docs",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := classifyPaths(tc.paths)
			if result.Classification != tc.wantClassification {
				t.Fatalf("classification = %q, want %q", result.Classification, tc.wantClassification)
			}
			if got := strings.Join(result.Areas, ","); got != tc.wantAreas {
				t.Fatalf("areas = %q, want %q", got, tc.wantAreas)
			}
		})
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
			name: "generated-api-surface",
			paths: []string{
				"pkg/api/generated/server.gen.go",
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
		{
			name: "root-build-config",
			paths: []string{
				"Makefile",
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

func TestLanePlansRouteNarrowAndSharedRiskClassifications(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		classification string
		wantRuns       map[string]bool
	}{
		{
			name:           "docs-only",
			classification: classificationDocsOnly,
			wantRuns: map[string]bool{
				"UI Coverage":            false,
				"UI Browser Integration": false,
				"Backend Verification":   false,
			},
		},
		{
			name:           "ui-only",
			classification: classificationUIOnly,
			wantRuns: map[string]bool{
				"UI Coverage":            true,
				"UI Browser Integration": true,
				"Backend Verification":   false,
			},
		},
		{
			name:           "backend-only",
			classification: classificationBackendOnly,
			wantRuns: map[string]bool{
				"UI Coverage":            false,
				"UI Browser Integration": false,
				"Backend Verification":   true,
			},
		},
		{
			name:           "shared-risk",
			classification: classificationSharedRisk,
			wantRuns: map[string]bool{
				"UI Coverage":            true,
				"UI Browser Integration": true,
				"Backend Verification":   true,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plans := lanePlans(tc.classification)
			if len(plans) != len(tc.wantRuns) {
				t.Fatalf("lanePlans(%q) returned %d plans, want %d", tc.classification, len(plans), len(tc.wantRuns))
			}
			for _, plan := range plans {
				want, ok := tc.wantRuns[plan.Name]
				if !ok {
					t.Fatalf("unexpected lane plan %q", plan.Name)
				}
				if plan.ShouldRun != want {
					t.Fatalf("lane %q shouldRun = %t, want %t", plan.Name, plan.ShouldRun, want)
				}
				if plan.Command == "" {
					t.Fatalf("lane %q command was empty", plan.Name)
				}
				if plan.Reason == "" {
					t.Fatalf("lane %q reason was empty", plan.Name)
				}
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
	if !strings.Contains(output, "run_ui_coverage=true") {
		t.Fatalf("GitHub output = %q, want UI coverage route output", output)
	}
	if !strings.Contains(output, "run_backend_verification=false") {
		t.Fatalf("GitHub output = %q, want backend verification route output", output)
	}
	if !strings.Contains(output, "backend_verification_reason=Skipped because UI-only changes do not require backend verification.") {
		t.Fatalf("GitHub output = %q, want backend skip reason", output)
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
	if strings.Contains(summary, "- Full required rerun: `make verify`") {
		t.Fatalf("GitHub summary = %q, did not want shared-risk rerun guidance for ui-only", summary)
	}
	if !strings.Contains(summary, "### Required lane routing") {
		t.Fatalf("GitHub summary = %q, want lane routing section", summary)
	}
	if !strings.Contains(summary, "- `Backend Verification`: `skip` via `make test-backend-verification`") {
		t.Fatalf("GitHub summary = %q, want backend skip line", summary)
	}
	if !strings.Contains(summary, "- `ui/src/App.tsx`") {
		t.Fatalf("GitHub summary = %q, want changed file list", summary)
	}
}

func TestRunWritesFullRunGuidanceForSharedRisk(t *testing.T) {
	tempDir := t.TempDir()
	changedFilesPath := filepath.Join(tempDir, "changed-files.txt")
	if err := os.WriteFile(changedFilesPath, []byte("Makefile\n"), 0o644); err != nil {
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

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read GitHub output: %v", err)
	}
	output := string(outputBytes)
	if !strings.Contains(output, "classification=shared-risk") {
		t.Fatalf("GitHub output = %q, want shared-risk classification", output)
	}
	if !strings.Contains(output, "full_run_required=true") {
		t.Fatalf("GitHub output = %q, want full_run_required", output)
	}
	if !strings.Contains(output, "full_run_command=make verify") {
		t.Fatalf("GitHub output = %q, want full run command", output)
	}

	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read GitHub summary: %v", err)
	}
	summary := string(summaryBytes)
	if !strings.Contains(summary, "- Full required rerun: `make verify`") {
		t.Fatalf("GitHub summary = %q, want full required rerun guidance", summary)
	}
}

func TestGitChangedPathsIncludesDeletedFilesInClassifierInput(t *testing.T) {
	t.Parallel()

	originalExecCommand := execCommand
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "git" {
			t.Fatalf("command = %q, want git", name)
		}
		wantArgs := []string{"diff", "--name-only", "--diff-filter=ACDMR", "origin/main", "HEAD"}
		if got := strings.Join(args, "\x00"); got != strings.Join(wantArgs, "\x00") {
			t.Fatalf("args = %#v, want %#v", args, wantArgs)
		}
		return exec.Command("sh", "-c", "printf 'docs/guide.md\\nui/src/App.tsx\\n'")
	}

	paths, err := gitChangedPaths("origin/main", "HEAD")
	if err != nil {
		t.Fatalf("gitChangedPaths() error = %v", err)
	}
	if got := strings.Join(paths, ","); got != "docs/guide.md,ui/src/App.tsx" {
		t.Fatalf("gitChangedPaths() = %q, want parsed deleted-path-capable output", got)
	}
}

func TestDevelopmentGuideDocumentsObservableCIRoutingContract(t *testing.T) {
	t.Parallel()

	guidePath := testpath.MustRepoPathFromCaller(t, 0, "docs", "internal", "development", "development.md")
	body, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", guidePath, err)
	}

	guide := string(body)
	wantSnippets := []string{
		"| `docs-only` | `docs/**` plus root-level docs or text files such as `README.md`, `*.md`, `*.mdx`, and `*.txt` | skip `UI Coverage`, skip `UI Browser Integration`, skip `Backend Verification` |",
		"| `ui-only` | `ui/**` plus optional documentation companions under `docs/**` or root-level `*.md`, `*.mdx`, and `*.txt` files | run `UI Coverage`, run `UI Browser Integration`, skip `Backend Verification` | `make test-ui-coverage` and `make ui-integration-test`",
		"| `backend-only` | `cmd/**`, `pkg/**`, or `tests/**` plus optional documentation companions under `docs/**` or root-level `*.md`, `*.mdx`, and `*.txt` files | skip `UI Coverage`, skip `UI Browser Integration`, run `Backend Verification` | `make test-backend-verification`",
		"| `shared-risk` | mixed product areas or explicit shared surfaces such as `.github/workflows/**`, `api/**`, `pkg/api/**`, `pkg/apisurface/**`, `Makefile`, `go.mod`, or `go.sum` | run `UI Coverage`, run `UI Browser Integration`, run `Backend Verification` | `make verify`",
		"| `UI Coverage` | `ui-coverage-failure-artifacts` | lane `command.log` with the failing command output |",
		"| `UI Browser Integration` | `ui-browser-integration-failure-artifacts` | lane `command.log` plus the shared harness browser evidence: Playwright trace, final screenshot, page HTML snapshot, and diagnostics JSON |",
		"| `Backend Verification` | `backend-verification-failure-artifacts` | lane `command.log` with the covered Go test and maintained short functional output |",
	}

	for _, snippet := range wantSnippets {
		if !strings.Contains(guide, snippet) {
			t.Fatalf("development guide missing CI routing contract snippet: %q", snippet)
		}
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
