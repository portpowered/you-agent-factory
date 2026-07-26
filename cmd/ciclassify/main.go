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

const fullRunCommand = "make verify-pr"

const (
	laneDocsReference            = "Docs Reference"
	laneReadme                   = "README"
	laneFrontend                 = "Frontend"
	laneBackend                  = "Backend"
	laneUIBackendIntegration     = "UI Backend Integration"
	laneAPIPackage               = "API Package"
	lanePackagedFactoriesPackage = "Packaged Factories Package"
	laneModelProvidersPackage    = "Model Providers Package"
	laneLocalInference           = "Local Inference"
)

var allLaneNames = []string{
	laneDocsReference,
	laneReadme,
	laneFrontend,
	laneBackend,
	laneUIBackendIntegration,
	laneAPIPackage,
	lanePackagedFactoriesPackage,
	laneModelProvidersPackage,
	laneLocalInference,
}

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
	Lanes          map[string]lanePlan
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
	return writeGitHubStepSummary(result)
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

func gitChangedPaths(baseRef, headRef string) ([]string, error) {
	output, err := execCommand("git", "diff", "--name-only", "--diff-filter=ACDMR", baseRef, headRef).Output()
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
		if path := strings.TrimSpace(scanner.Text()); path != "" {
			paths = append(paths, filepath.ToSlash(path))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read changed paths file %s: %w", filepath.ToSlash(path), err)
	}
	return dedupeAndSortPaths(paths), nil
}

func parseChangedPaths(raw string) []string { return dedupeAndSortPaths(strings.Fields(raw)) }

func dedupeAndSortPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if _, exists := seen[path]; !exists {
			seen[path] = struct{}{}
			result = append(result, path)
		}
	}
	slices.Sort(result)
	return result
}

func classifyPaths(paths []string) classificationResult {
	areas := map[string]bool{}
	lanes := newLanePlans()
	full := len(paths) == 0
	for _, path := range paths {
		area, selected := classifyPath(path)
		areas[area] = true
		if area == "unknown" || area == "ci-tooling" {
			full = true
		}
		for _, lane := range selected {
			plan := lanes[lane]
			plan.ShouldRun = true
			lanes[lane] = plan
		}
	}
	if full {
		for name, plan := range lanes {
			plan.ShouldRun = true
			lanes[name] = plan
		}
	}
	areaNames := mapKeysSorted(areas)
	classification := "none"
	reason := "Only factory content changed, so no product verification lane is selected."
	if full {
		classification = "full"
		reason = "A CI/tooling, unknown, or empty change set requires conservative full verification."
	} else if len(areaNames) > 0 {
		classification = strings.Join(areaNames, "+")
		reason = "Selected the union of verification lanes owned by the changed paths."
	}
	for name, plan := range lanes {
		if !plan.ShouldRun {
			plan.Reason = "Skipped because no changed path selected this owned verification lane."
			lanes[name] = plan
		}
	}
	return classificationResult{Classification: classification, Areas: areaNames, ChangedPaths: paths, Reason: reason, Lanes: lanes}
}

func classifyPath(path string) (string, []string) {
	switch {
	case strings.HasPrefix(path, ".github/workflows/"), strings.HasPrefix(path, "scripts/ci/"), path == "Makefile", path == "go.mod", path == "go.sum":
		return "ci-tooling", nil
	case path == "README.md":
		return "readme", []string{laneReadme}
	case strings.HasPrefix(path, "docs/reference/"), path == "docs/README.md":
		return "documentation-reference", []string{laneDocsReference}
	case strings.HasPrefix(path, "factory/"):
		return "factory-content", nil
	case strings.HasPrefix(path, "api/"), strings.HasPrefix(path, "contracts/"), strings.HasPrefix(path, "pkg/transports/http/"), strings.HasPrefix(path, "pkg/transports/mapping/"), strings.HasPrefix(path, "ui/src/api/generated/"):
		return "api-contract", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}
	case strings.HasPrefix(path, "packages/api/"):
		return "api-package", []string{laneAPIPackage, laneFrontend, laneBackend, laneUIBackendIntegration}
	case strings.HasPrefix(path, "packages/packaged-factories/"):
		return "packaged-factories-package", []string{lanePackagedFactoriesPackage, laneBackend}
	case strings.HasPrefix(path, "packages/model-providers/"):
		return "model-providers-package", []string{laneModelProvidersPackage, laneBackend}
	case isLocalInferencePath(path):
		return "local-inference", []string{laneBackend, laneLocalInference}
	case strings.HasPrefix(path, "ui/"):
		return "frontend", []string{laneFrontend}
	case strings.HasPrefix(path, "pkg/"), strings.HasPrefix(path, "cmd/"), strings.HasPrefix(path, "internal/"), strings.HasPrefix(path, "tests/"):
		return "backend", []string{laneBackend, laneUIBackendIntegration}
	default:
		return "unknown", nil
	}
}

func isLocalInferencePath(path string) bool {
	return strings.Contains(path, "/local/") || strings.Contains(path, "inference") || strings.Contains(path, "omnivoice") || strings.HasPrefix(path, ".github/workflows/long-local-inference")
}

func newLanePlans() map[string]lanePlan {
	return map[string]lanePlan{
		laneDocsReference:            {Name: laneDocsReference, Command: "make docs-reference-smoke"},
		laneReadme:                   {Name: laneReadme, Command: "make readme-check"},
		laneFrontend:                 {Name: laneFrontend, Command: "make typecheck ui-lint test-ui-coverage test-ui-browser-integration"},
		laneBackend:                  {Name: laneBackend, Command: "make build test-backend-verification"},
		laneUIBackendIntegration:     {Name: laneUIBackendIntegration, Command: "make ui-durable-session-real-backend-integration-test"},
		laneAPIPackage:               {Name: laneAPIPackage, Command: "make api-smoke"},
		lanePackagedFactoriesPackage: {Name: lanePackagedFactoriesPackage, Command: "make packaged-factories-package-verify"},
		laneModelProvidersPackage:    {Name: laneModelProvidersPackage, Command: "make model-providers-package-verify"},
		laneLocalInference:           {Name: laneLocalInference, Command: "make verify-pr-inference"},
	}
}

func mapKeysSorted(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func writeStdoutSummary(stdout io.Writer, result classificationResult) {
	fmt.Fprintf(stdout, "classification=%s\nareas=%s\nchanged_files=%d\nreason=%s\n", result.Classification, strings.Join(result.Areas, ","), len(result.ChangedPaths), result.Reason)
	for _, name := range allLaneNames {
		plan := result.Lanes[name]
		fmt.Fprintf(stdout, "lane_%s=%t\n", githubOutputKey(plan.Name), plan.ShouldRun)
	}
}

func githubOutputKey(name string) string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(name))
}

func writeGitHubOutput(result classificationResult) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	lines := []string{fmt.Sprintf("classification=%s", result.Classification), fmt.Sprintf("full_run_required=%t", result.Classification == "full"), fmt.Sprintf("full_run_command=%s", fullRunCommand), fmt.Sprintf("areas=%s", strings.Join(result.Areas, ",")), fmt.Sprintf("changed_files_count=%d", len(result.ChangedPaths)), fmt.Sprintf("reason=%s", sanitizeGitHubValue(result.Reason))}
	for _, name := range allLaneNames {
		plan := result.Lanes[name]
		key := githubOutputKey(name)
		lines = append(lines, fmt.Sprintf("run_%s=%t", key, plan.ShouldRun), fmt.Sprintf("%s_reason=%s", key, sanitizeGitHubValue(plan.Reason)), fmt.Sprintf("%s_command=%s", key, plan.Command))
	}
	return appendFile(path, strings.Join(append(lines, ""), "\n"), "write GitHub output")
}

func writeGitHubStepSummary(result classificationResult) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	lines := []string{"## Pull request impact classification", "", fmt.Sprintf("- Classification: `%s`", result.Classification), fmt.Sprintf("- Areas touched: `%s`", strings.Join(result.Areas, ", ")), fmt.Sprintf("- Changed files: `%d`", len(result.ChangedPaths)), fmt.Sprintf("- Reason: %s", result.Reason), fmt.Sprintf("- Full required rerun: `%s`", fullRunCommand), "", "### Verification policy"}
	for _, name := range allLaneNames {
		plan := result.Lanes[name]
		decision := "skip"
		if plan.ShouldRun {
			decision = "run"
		}
		lines = append(lines, fmt.Sprintf("- `%s`: `%s` via `%s`", plan.Name, decision, plan.Command), "  "+plan.Reason)
	}
	return appendFile(path, strings.Join(append(lines, ""), "\n"), "write GitHub step summary")
}

func appendFile(path, content, operation string) error {
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

func sanitizeGitHubValue(value string) string { return strings.ReplaceAll(value, "\n", " ") }
