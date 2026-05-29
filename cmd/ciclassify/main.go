package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	classificationDocsOnly    = "docs-only"
	classificationUIOnly      = "ui-only"
	classificationBackendOnly = "backend-only"
	classificationSharedRisk  = "shared-risk"
	fullRunCommand            = "make verify"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
	execCommand            = exec.Command
)

type config struct {
	baseRef          string
	headRef          string
	changedFilesPath string
}

type classificationResult struct {
	Classification string
	Areas          []string
	ChangedPaths   []string
	Reason         string
}

type lanePlan struct {
	Name      string
	Command   string
	ShouldRun bool
	Reason    string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.baseRef, "base", "", "git base ref or sha to diff from")
	flag.StringVar(&cfg.headRef, "head", "", "git head ref or sha to diff to")
	flag.StringVar(&cfg.changedFilesPath, "changed-files-path", "", "optional newline-delimited file of changed paths")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, _ io.Writer) error {
	changedPaths, err := resolveChangedPaths(cfg)
	if err != nil {
		return err
	}

	result := classifyPaths(changedPaths)
	writeStdoutSummary(stdout, result)
	if err := writeGitHubOutput(result); err != nil {
		return err
	}
	if err := writeGitHubStepSummary(result); err != nil {
		return err
	}
	return nil
}

func resolveChangedPaths(cfg config) ([]string, error) {
	switch {
	case cfg.changedFilesPath != "":
		return readChangedPathsFile(cfg.changedFilesPath)
	case cfg.baseRef != "" && cfg.headRef != "":
		return gitChangedPaths(cfg.baseRef, cfg.headRef)
	case cfg.baseRef == "" && cfg.headRef == "":
		return nil, fmt.Errorf("either -changed-files-path or both -base and -head must be provided")
	default:
		return nil, fmt.Errorf("both -base and -head must be provided together")
	}
}

func gitChangedPaths(baseRef string, headRef string) ([]string, error) {
	cmd := execCommand("git", "diff", "--name-only", "--diff-filter=ACDMR", baseRef, headRef)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read changed paths from git diff %s..%s: %w", baseRef, headRef, err)
	}
	return parseChangedPaths(string(output)), nil
}

func readChangedPathsFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open changed paths file %s: %w", filepath.ToSlash(path), err)
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read changed paths file %s: %w", filepath.ToSlash(path), err)
	}
	return dedupeAndSortPaths(paths), nil
}

func parseChangedPaths(raw string) []string {
	return dedupeAndSortPaths(strings.Fields(raw))
}

func dedupeAndSortPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	deduped := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		deduped = append(deduped, normalized)
	}
	slices.Sort(deduped)
	return deduped
}

func classifyPaths(paths []string) classificationResult {
	if len(paths) == 0 {
		return classificationResult{
			Classification: classificationSharedRisk,
			Areas:          []string{"shared-risk"},
			ChangedPaths:   nil,
			Reason:         "No changed files were detected, so the workflow falls back to the explicit shared-risk full verification path.",
		}
	}

	areasSeen := map[string]struct{}{}
	for _, path := range paths {
		areasSeen[classifyPath(path)] = struct{}{}
	}

	areas := mapsKeysSorted(areasSeen)
	result := classificationResult{
		Areas:        areas,
		ChangedPaths: paths,
	}

	switch {
	case len(areasSeen) == 1 && hasArea(areasSeen, "docs"):
		result.Classification = classificationDocsOnly
		result.Reason = "Only documentation-owned files changed, so later jobs can treat this pull request as documentation-only."
	case onlyAreas(areasSeen, "docs", "ui") && hasArea(areasSeen, "ui"):
		result.Classification = classificationUIOnly
		result.Reason = "Only UI-owned files and optional documentation changed, so this pull request stays on the UI-only path."
	case onlyAreas(areasSeen, "docs", "backend") && hasArea(areasSeen, "backend"):
		result.Classification = classificationBackendOnly
		result.Reason = "Only backend-owned files and optional documentation changed, so this pull request stays on the backend-only path."
	default:
		result.Classification = classificationSharedRisk
		result.Reason = "The changed files cross product boundaries or touch explicit shared-risk surfaces such as workflows, contracts, generated API boundaries, or root build configuration, so the workflow keeps the full verification safety path."
	}
	return result
}

func classifyPath(path string) string {
	if isSharedRiskPath(path) {
		return "shared-risk"
	}
	if isDocumentationPath(path) {
		return "docs"
	}
	if strings.HasPrefix(path, "ui/") {
		return "ui"
	}
	if strings.HasPrefix(path, "pkg/") || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "tests/") {
		return "backend"
	}
	return "shared-risk"
}

func isSharedRiskPath(path string) bool {
	if strings.HasPrefix(path, ".github/workflows/") {
		return true
	}
	if strings.HasPrefix(path, "api/") {
		return true
	}
	if strings.HasPrefix(path, "pkg/api/") || strings.HasPrefix(path, "pkg/apisurface/") {
		return true
	}

	switch path {
	case "Makefile", "go.mod", "go.sum":
		return true
	default:
		return false
	}
}

func isDocumentationPath(path string) bool {
	if strings.HasPrefix(path, "docs/") {
		return true
	}
	if strings.Contains(path, "/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx" || ext == ".txt"
}

func mapsKeysSorted(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func hasArea(areas map[string]struct{}, area string) bool {
	_, ok := areas[area]
	return ok
}

func onlyAreas(areas map[string]struct{}, allowed ...string) bool {
	if len(areas) == 0 {
		return false
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, area := range allowed {
		allowedSet[area] = struct{}{}
	}
	for area := range areas {
		if _, ok := allowedSet[area]; !ok {
			return false
		}
	}
	return true
}

func writeStdoutSummary(stdout io.Writer, result classificationResult) {
	fmt.Fprintf(stdout, "classification=%s\n", result.Classification)
	fmt.Fprintf(stdout, "areas=%s\n", strings.Join(result.Areas, ","))
	fmt.Fprintf(stdout, "changed_files=%d\n", len(result.ChangedPaths))
	fmt.Fprintf(stdout, "reason=%s\n", result.Reason)
	for _, plan := range lanePlans(result.Classification) {
		fmt.Fprintf(stdout, "lane_%s=%t\n", githubOutputKey(plan.Name), plan.ShouldRun)
	}
}

func lanePlans(classification string) []lanePlan {
	switch classification {
	case classificationDocsOnly:
		return []lanePlan{
			{
				Name:      "UI Coverage",
				Command:   "make test-ui-coverage",
				ShouldRun: false,
				Reason:    "Skipped because documentation-only changes do not require UI coverage.",
			},
			{
				Name:      "UI Browser Integration",
				Command:   "make ui-integration-test",
				ShouldRun: false,
				Reason:    "Skipped because documentation-only changes do not require UI browser integration.",
			},
			{
				Name:      "Backend Verification",
				Command:   "make test-backend-verification",
				ShouldRun: false,
				Reason:    "Skipped because documentation-only changes do not require backend verification.",
			},
		}
	case classificationUIOnly:
		return []lanePlan{
			{
				Name:      "UI Coverage",
				Command:   "make test-ui-coverage",
				ShouldRun: true,
				Reason:    "Runs because UI-only changes still require the owned UI coverage lane.",
			},
			{
				Name:      "UI Browser Integration",
				Command:   "make ui-integration-test",
				ShouldRun: true,
				Reason:    "Runs because UI-only changes still require browser-backed UI verification.",
			},
			{
				Name:      "Backend Verification",
				Command:   "make test-backend-verification",
				ShouldRun: false,
				Reason:    "Skipped because UI-only changes do not require backend verification.",
			},
		}
	case classificationBackendOnly:
		return []lanePlan{
			{
				Name:      "UI Coverage",
				Command:   "make test-ui-coverage",
				ShouldRun: false,
				Reason:    "Skipped because backend-only changes do not require UI coverage.",
			},
			{
				Name:      "UI Browser Integration",
				Command:   "make ui-integration-test",
				ShouldRun: false,
				Reason:    "Skipped because backend-only changes do not require UI browser integration.",
			},
			{
				Name:      "Backend Verification",
				Command:   "make test-backend-verification",
				ShouldRun: true,
				Reason:    "Runs because backend-only changes still require backend verification.",
			},
		}
	default:
		return []lanePlan{
			{
				Name:      "UI Coverage",
				Command:   "make test-ui-coverage",
				ShouldRun: true,
				Reason:    "Runs because shared-risk changes stay on the full verification safety path.",
			},
			{
				Name:      "UI Browser Integration",
				Command:   "make ui-integration-test",
				ShouldRun: true,
				Reason:    "Runs because shared-risk changes stay on the full verification safety path.",
			},
			{
				Name:      "Backend Verification",
				Command:   "make test-backend-verification",
				ShouldRun: true,
				Reason:    "Runs because shared-risk changes stay on the full verification safety path.",
			},
		}
	}
}

func githubOutputKey(name string) string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(name))
}

func writeGitHubOutput(result classificationResult) error {
	outputPath := os.Getenv("GITHUB_OUTPUT")
	if outputPath == "" {
		return nil
	}

	lines := []string{
		fmt.Sprintf("classification=%s", result.Classification),
		fmt.Sprintf("docs_only=%t", result.Classification == classificationDocsOnly),
		fmt.Sprintf("ui_only=%t", result.Classification == classificationUIOnly),
		fmt.Sprintf("backend_only=%t", result.Classification == classificationBackendOnly),
		fmt.Sprintf("shared_risk=%t", result.Classification == classificationSharedRisk),
		fmt.Sprintf("full_run_required=%t", result.Classification == classificationSharedRisk),
		fmt.Sprintf("full_run_command=%s", fullRunCommand),
		fmt.Sprintf("areas=%s", strings.Join(result.Areas, ",")),
		fmt.Sprintf("changed_files_count=%d", len(result.ChangedPaths)),
		fmt.Sprintf("reason=%s", sanitizeGitHubValue(result.Reason)),
	}
	for _, plan := range lanePlans(result.Classification) {
		key := githubOutputKey(plan.Name)
		lines = append(lines,
			fmt.Sprintf("run_%s=%t", key, plan.ShouldRun),
			fmt.Sprintf("%s_reason=%s", key, sanitizeGitHubValue(plan.Reason)),
			fmt.Sprintf("%s_command=%s", key, plan.Command),
		)
	}
	lines = append(lines, "")
	return appendFile(outputPath, strings.Join(lines, "\n"), "write GitHub output")
}

func writeGitHubStepSummary(result classificationResult) error {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return nil
	}

	lines := []string{
		"## Pull request impact classification",
		"",
		fmt.Sprintf("- Classification: `%s`", result.Classification),
		fmt.Sprintf("- Areas touched: `%s`", strings.Join(result.Areas, ", ")),
		fmt.Sprintf("- Changed files: `%d`", len(result.ChangedPaths)),
		fmt.Sprintf("- Reason: %s", result.Reason),
		fmt.Sprintf("- Full required rerun: `%s`", fullRunCommand),
	}
	lines = append(lines, "", "### Required lane routing")
	for _, plan := range lanePlans(result.Classification) {
		decision := "skip"
		if plan.ShouldRun {
			decision = "run"
		}
		lines = append(lines, fmt.Sprintf("- `%s`: `%s` via `%s`", plan.Name, decision, plan.Command))
		lines = append(lines, fmt.Sprintf("  %s", plan.Reason))
	}
	if len(result.ChangedPaths) > 0 {
		lines = append(lines, "", "### Changed files")
		limit := min(len(result.ChangedPaths), 20)
		for _, path := range result.ChangedPaths[:limit] {
			lines = append(lines, fmt.Sprintf("- `%s`", path))
		}
		if len(result.ChangedPaths) > limit {
			lines = append(lines, fmt.Sprintf("- `%d` additional files omitted from the summary", len(result.ChangedPaths)-limit))
		}
	}
	lines = append(lines, "")
	return appendFile(summaryPath, strings.Join(lines, "\n"), "write GitHub step summary")
}

func appendFile(path string, content string, operation string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("%s %s: %w", operation, filepath.ToSlash(path), err)
	}
	defer file.Close()

	if _, err := io.WriteString(file, content); err != nil {
		return fmt.Errorf("%s %s: %w", operation, filepath.ToSlash(path), err)
	}
	return nil
}

func sanitizeGitHubValue(value string) string {
	return strings.ReplaceAll(value, "\n", " ")
}
