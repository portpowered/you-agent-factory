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
	if !strings.Contains(summary, "- Full required rerun: `make verify-pr`") {
		t.Fatalf("GitHub summary = %q, want full rerun guidance for skipped backend verification", summary)
	}
	if !strings.Contains(summary, "### Required lane routing") {
		t.Fatalf("GitHub summary = %q, want lane routing section", summary)
	}
	if !strings.Contains(summary, "- `Backend Verification`: `skip` via `make test-backend-verification`") {
		t.Fatalf("GitHub summary = %q, want backend skip line", summary)
	}
	if !strings.Contains(summary, "Skipped because UI-only changes do not require backend verification.") {
		t.Fatalf("GitHub summary = %q, want backend skip reason", summary)
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
	if !strings.Contains(output, "full_run_command=make verify-pr") {
		t.Fatalf("GitHub output = %q, want full run command", output)
	}

	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read GitHub summary: %v", err)
	}
	summary := string(summaryBytes)
	if !strings.Contains(summary, "- Full required rerun: `make verify-pr`") {
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

func TestDevelopmentGuideMatchesObservableCIRoutingContract(t *testing.T) {
	guidePath := testpath.MustRepoPathFromCaller(t, 0, "docs", "internal", "development", "development.md")
	body, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", guidePath, err)
	}

	guide := string(body)
	classificationRows := mustParseMarkdownTable(t, guide, "| Classification | Touched surfaces | Required downstream lanes | Local rerun guidance |")
	laneRows := mustParseMarkdownTable(t, guide, "| CI lane | Owned checks | Local rerun command | Why this lane stays separate |")

	scenarios := []struct {
		name               string
		paths              []string
		wantClassification string
	}{
		{
			name:               "docs-only",
			paths:              []string{"docs/internal/development/development.md", "README.md"},
			wantClassification: classificationDocsOnly,
		},
		{
			name:               "ui-only",
			paths:              []string{"ui/src/App.tsx", "docs/internal/development/development.md"},
			wantClassification: classificationUIOnly,
		},
		{
			name:               "backend-only",
			paths:              []string{"pkg/service/server.go", "docs/internal/development/development.md"},
			wantClassification: classificationBackendOnly,
		},
		{
			name:               "shared-risk",
			paths:              []string{"Makefile"},
			wantClassification: classificationSharedRisk,
		},
	}

	for _, scenario := range scenarios {
		summary := observeClassifierSummary(t, scenario.paths)
		row := mustFindMarkdownRow(t, classificationRows, "`"+scenario.wantClassification+"`")
		if got := row[2]; got != expectedGuideLaneMatrix(scenario.wantClassification) {
			t.Fatalf("%s guide lanes = %q, want %q", scenario.wantClassification, got, expectedGuideLaneMatrix(scenario.wantClassification))
		}
		for _, want := range expectedGuideRerunGuidance(scenario.wantClassification) {
			if !strings.Contains(row[3], want) {
				t.Fatalf("%s guide rerun guidance = %q, want to contain %q", scenario.wantClassification, row[3], want)
			}
		}
		if !strings.Contains(summary, "- Classification: `"+scenario.wantClassification+"`") {
			t.Fatalf("%s summary = %q, want classification line", scenario.wantClassification, summary)
		}
		for _, want := range expectedSummaryLaneLines(scenario.wantClassification) {
			if !strings.Contains(summary, want) {
				t.Fatalf("%s summary = %q, want lane line %q", scenario.wantClassification, summary, want)
			}
		}
	}

	for _, plan := range lanePlans(classificationSharedRisk) {
		row := mustFindMarkdownRow(t, laneRows, "`"+plan.Name+"`")
		if got := row[2]; got != "`"+plan.Command+"`" {
			t.Fatalf("%s guide rerun command = %q, want %q", plan.Name, got, "`"+plan.Command+"`")
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

func observeClassifierSummary(t *testing.T, paths []string) string {
	t.Helper()

	tempDir := t.TempDir()
	changedFilesPath := filepath.Join(tempDir, "changed-files.txt")
	if err := os.WriteFile(changedFilesPath, []byte(strings.Join(paths, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write changed files fixture: %v", err)
	}

	outputPath := filepath.Join(tempDir, "github-output.txt")
	summaryPath := filepath.Join(tempDir, "github-summary.md")

	t.Setenv("GITHUB_OUTPUT", outputPath)
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	stdout := &bytes.Buffer{}
	if err := run(config{changedFilesPath: changedFilesPath}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read GitHub summary: %v", err)
	}
	return string(summaryBytes)
}

func mustParseMarkdownTable(t *testing.T, document string, header string) [][]string {
	t.Helper()

	start := strings.Index(document, header)
	if start < 0 {
		t.Fatalf("markdown table header %q not found", header)
	}

	lines := strings.Split(document[start:], "\n")
	if len(lines) < 3 {
		t.Fatalf("markdown table for %q was truncated", header)
	}

	rows := make([][]string, 0)
	for _, line := range lines[2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		rows = append(rows, splitMarkdownRow(line))
	}
	if len(rows) == 0 {
		t.Fatalf("markdown table %q had no data rows", header)
	}
	return rows
}

func mustFindMarkdownRow(t *testing.T, rows [][]string, firstColumn string) []string {
	t.Helper()

	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if row[0] == firstColumn {
			return row
		}
	}
	t.Fatalf("markdown row %q not found", firstColumn)
	return nil
}

func splitMarkdownRow(line string) []string {
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts)-2)
	for _, part := range parts[1 : len(parts)-1] {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func expectedGuideLaneMatrix(classification string) string {
	parts := make([]string, 0, len(lanePlans(classification)))
	for _, plan := range lanePlans(classification) {
		decision := "skip"
		if plan.ShouldRun {
			decision = "run"
		}
		parts = append(parts, decision+" `"+plan.Name+"`")
	}
	return strings.Join(parts, ", ")
}

func expectedGuideRerunGuidance(classification string) []string {
	switch classification {
	case classificationDocsOnly:
		return []string{"`go run ./cmd/ciclassify ...`", "`make verify-pr`"}
	case classificationSharedRisk:
		return []string{"`" + fullRunCommand + "`"}
	default:
		commands := make([]string, 0, len(lanePlans(classification)))
		for _, plan := range lanePlans(classification) {
			if plan.ShouldRun {
				commands = append(commands, "`"+plan.Command+"`")
			}
		}
		return commands
	}
}

func expectedSummaryLaneLines(classification string) []string {
	lines := make([]string, 0, len(lanePlans(classification)))
	for _, plan := range lanePlans(classification) {
		decision := "skip"
		if plan.ShouldRun {
			decision = "run"
		}
		lines = append(lines, "- `"+plan.Name+"`: `"+decision+"` via `"+plan.Command+"`")
	}
	return lines
}
