package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	totalCoveragePattern   = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)
	packageCoveragePattern = regexp.MustCompile(`^(?:ok\s+)?(\S+)(?:\s+\S+)*\s+coverage:\s+([0-9.]+)% of statements(?:\s+in\s+.+)?$`)
)

const modulePath = "github.com/portpowered/infinite-you"
const defaultPackageCoverageBaselinePath = "docs/internal/development/go-coverage-package-baseline.txt"
const defaultPackageCoverageMin = 80.0

var (
	defaultCoveragePatterns           = []string{"./cmd/factory", "./pkg/..."}
	defaultTestPatterns               = []string{"./cmd/factory", "./pkg/...", "./tests/functional/..."}
	execCommand                       = exec.Command
	stdoutWriter            io.Writer = os.Stdout
	stderrWriter            io.Writer = os.Stderr
	exitFunc                          = os.Exit
)

type config struct {
	covermode       string
	coverpkg        string
	min             float64
	packageBaseline string
	packageMin      float64
	packages        string
	profile         string
	short           bool
	timeout         time.Duration
}

type coverageResult struct {
	actual                       float64
	insufficientCoveragePackages []packageCoverageSummary
	packageSummaries             []packageCoverageSummary
	zeroCoveragePackages         []string
}

type packageCoverageTotals struct {
	coveredStatements int
	totalStatements   int
}

type packageCoverageSummary struct {
	importPath string
	coverage   float64
}

type coverageBlock struct {
	importPath     string
	statementCount int
	executionCount int
}

func main() {
	cfg := parseConfig()
	if err := execute(cfg); err != nil {
		failf("%v\n", err)
	}
}

func execute(cfg config) error {
	result, err := run(cfg)
	if err != nil {
		return err
	}

	var failures []string
	if result.actual < cfg.min {
		failures = append(failures, fmt.Sprintf("go coverage %.1f%% is below minimum %.1f%%", result.actual, cfg.min))
	}
	if len(result.insufficientCoveragePackages) > 0 {
		failures = append(failures, formatInsufficientCoverageFailure(result.insufficientCoveragePackages, cfg.packageCoverageMin()))
	}

	for _, summary := range result.packageSummaries {
		fmt.Fprintf(stdoutWriter, "%s\tcoverage: %.1f%% of statements\n", summary.importPath, summary.coverage)
	}

	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	fmt.Fprintf(stdoutWriter, "Go coverage %.1f%% meets minimum %.1f%%.\n", result.actual, cfg.min)
	return nil
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.covermode, "covermode", "count", "go test -covermode value")
	flag.StringVar(&cfg.coverpkg, "coverpkg", "", "comma-separated import paths to measure; defaults to backend-owned packages")
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packageBaseline, "package-baseline", defaultPackageCoverageBaselinePath, "newline-delimited list of backend packages temporarily exempt from the per-package minimum coverage gate")
	flag.Float64Var(&cfg.packageMin, "package-min", defaultPackageCoverageMin, "minimum statement coverage required for each non-baselined backend package")
	flag.StringVar(&cfg.packages, "packages", "", "space-separated go test package patterns; defaults to backend package tests plus backend-facing functional tests")
	flag.StringVar(&cfg.profile, "profile", "", "coverage profile output path; defaults to a temp file")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.Parse()
	return cfg
}

func (cfg config) packageCoverageBaselinePath() string {
	if strings.TrimSpace(cfg.packageBaseline) == "" {
		return defaultPackageCoverageBaselinePath
	}
	return cfg.packageBaseline
}

func (cfg config) packageCoverageMin() float64 {
	if cfg.packageMin <= 0 {
		return defaultPackageCoverageMin
	}
	return cfg.packageMin
}

func run(cfg config) (coverageResult, error) {
	profilePath := cfg.profile
	cleanup := func() error { return nil }
	if profilePath == "" {
		file, err := os.CreateTemp("", "go-coverage-*.out")
		if err != nil {
			return coverageResult{}, fmt.Errorf("create temp coverage profile: %w", err)
		}
		profilePath = file.Name()
		if err := file.Close(); err != nil {
			return coverageResult{}, fmt.Errorf("close temp coverage profile: %w", err)
		}
		cleanup = func() error {
			return os.Remove(profilePath)
		}
	}
	defer func() {
		_ = cleanup()
	}()

	coverPackages, testPackages, err := resolveCoverageLane(cfg)
	if err != nil {
		return coverageResult{}, err
	}
	selectedPackages := selectedCoveragePackages(coverPackages)

	mergedTestArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", strings.Join(coverPackages, ",")),
	}
	if cfg.short {
		mergedTestArgs = append(mergedTestArgs, "-short")
	}
	mergedTestArgs = append(mergedTestArgs,
		fmt.Sprintf("-coverprofile=%s", profilePath),
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)
	mergedTestArgs = append(mergedTestArgs, testPackages...)

	_, _, err = runGoTestCoverageLane(mergedTestArgs, "run go test coverage lane")
	if err != nil {
		return coverageResult{}, err
	}

	localPackageTestArgs := []string{
		"test",
		"-cover",
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	}
	if cfg.short {
		localPackageTestArgs = append(localPackageTestArgs, "-short")
	}
	localPackageTestArgs = append(localPackageTestArgs, selectedPackages...)

	localPackageStdout, localPackageStderr, err := runGoTestCoverageLane(localPackageTestArgs, "run go test package coverage lane")
	if err != nil {
		return coverageResult{}, err
	}

	coverCmd := execCommand("go", "tool", "cover", "-func", profilePath)
	coverCmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	coverCmd.Stdout = &stdout
	coverCmd.Stderr = &stderr
	if err := coverCmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return coverageResult{}, fmt.Errorf("summarize go coverage: %w\n%s", err, detail)
		}
		return coverageResult{}, fmt.Errorf("summarize go coverage: %w", err)
	}

	repoRoot, err := repoRootDir()
	if err != nil {
		return coverageResult{}, err
	}
	localPackageSummaryReport := localPackageStdout
	if strings.TrimSpace(localPackageStderr) != "" {
		localPackageSummaryReport += "\n" + localPackageStderr
	}

	baselinePackages, err := packageCoverageBaselinePackages(cfg, repoRoot)
	if err != nil {
		return coverageResult{}, err
	}

	result, totalLine, err := evaluateCoverage(stdout.String(), localPackageSummaryReport, profilePath, repoRoot, coverPackages, cfg.packageCoverageMin(), baselinePackages)
	if err != nil {
		return coverageResult{}, err
	}
	fmt.Fprintln(stdoutWriter, totalLine)
	return result, nil
}

func packageCoverageBaselinePackages(cfg config, repoRoot string) (map[string]struct{}, error) {
	baselinePath := cfg.packageCoverageBaselinePath()
	if !filepath.IsAbs(baselinePath) {
		baselinePath = filepath.Join(repoRoot, baselinePath)
	}
	return readPackageCoverageBaseline(baselinePath)
}

func runGoTestCoverageLane(args []string, failurePrefix string) (string, string, error) {
	testCmd := execCommand("go", args...)
	testCmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	testCmd.Stdout = &stdout
	testCmd.Stderr = &stderr
	if err := testCmd.Run(); err != nil {
		detail := mergeGoTestFailureDetail(stderr.String(), stdout.String())
		if detail != "" {
			return "", "", fmt.Errorf("%s: %w\n%s", failurePrefix, err, detail)
		}
		return "", "", fmt.Errorf("%s: %w", failurePrefix, err)
	}
	return stdout.String(), stderr.String(), nil
}

func mergeGoTestFailureDetail(stderr string, stdout string) string {
	stderr = strings.TrimSpace(stderr)
	stdout = strings.TrimSpace(stdout)
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	case strings.Contains(stdout, "\nFAIL") || strings.Contains(stdout, "--- FAIL:"):
		return stdout + "\n" + stderr
	default:
		return stderr + "\n" + stdout
	}
}

func resolveCoverageLane(cfg config) ([]string, []string, error) {
	coverPackages, err := resolveCoverPackages(cfg)
	if err != nil {
		return nil, nil, err
	}
	testPackages, err := resolveTestPackages(cfg)
	if err != nil {
		return nil, nil, err
	}
	return coverPackages, testPackages, nil
}

func resolveCoverPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.coverpkg) != "" {
		return splitList(cfg.coverpkg, ",", false), nil
	}
	return listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
}

func resolveTestPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.packages) != "" {
		return splitList(cfg.packages, " ", true), nil
	}
	return listGoPackages(defaultTestPatterns, isBackendTestPackage, false)
}

func listGoPackages(patterns []string, include func(string) bool, requireNonTestGoFiles bool) ([]string, error) {
	args := append([]string{"list", "-f", "{{.ImportPath}}\t{{len .GoFiles}}"}, patterns...)
	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	rootDir, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	cmd.Dir = rootDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("list go packages: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("list go packages: %w", err)
	}

	seen := make(map[string]struct{})
	var packages []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		importPath, goFiles, hasGoFiles := parseGoListPackageLine(line)
		if importPath == "" || !include(importPath) {
			continue
		}
		if requireNonTestGoFiles && hasGoFiles && goFiles == 0 {
			continue
		}
		if _, ok := seen[importPath]; ok {
			continue
		}
		seen[importPath] = struct{}{}
		packages = append(packages, importPath)
	}
	slices.Sort(packages)
	if len(packages) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}
	return packages, nil
}

func parseGoListPackageLine(line string) (string, int, bool) {
	fields := strings.Split(strings.TrimSpace(line), "\t")
	if len(fields) < 2 {
		return strings.TrimSpace(line), 0, false
	}
	goFiles, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return strings.TrimSpace(fields[0]), 0, false
	}
	return strings.TrimSpace(fields[0]), goFiles, true
}

func repoRootDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("resolve repository root: go.mod not found")
		}
		dir = parent
	}
}

func isBackendCoveragePackage(importPath string) bool {
	switch {
	case importPath == modulePath+"/cmd/factory":
		return true
	case !strings.HasPrefix(importPath, modulePath+"/pkg/"):
		return false
	case importPath == modulePath+"/pkg/transports/http/generated":
		return false
	case importPath == modulePath+"/pkg/transports/http/client":
		return false
	case importPath == modulePath+"/pkg/transports/mcp/generated":
		return false
	case strings.HasPrefix(importPath, modulePath+"/pkg/testutil"):
		return false
	default:
		return true
	}
}

func isBackendTestPackage(importPath string) bool {
	if isBackendCoveragePackage(importPath) {
		return true
	}
	return strings.HasPrefix(importPath, modulePath+"/tests/functional/") &&
		!strings.HasPrefix(importPath, modulePath+"/tests/functional/internal/")
}

func splitList(value string, separator string, filterEmpty bool) []string {
	parts := strings.Split(value, separator)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" && filterEmpty {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}

func parseTotalCoverage(report string) (float64, string, error) {
	matches := totalCoveragePattern.FindStringSubmatch(report)
	if len(matches) != 2 {
		return 0, "", errors.New("parse go coverage total: missing total statements line")
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse go coverage percentage %q: %w", matches[1], err)
	}
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "total:") {
			return value, fmt.Sprintf("total: (statements) %.1f%%", value), nil
		}
	}
	return value, fmt.Sprintf("total: (statements) %.1f%%", value), nil
}

func evaluateCoverage(_ string, packageSummaryReport string, profilePath string, repoRoot string, coverPackages []string, minCoverage float64, baselinePackages map[string]struct{}) (coverageResult, string, error) {
	packageTotals, err := readCoverageProfileTotals(profilePath, repoRoot)
	if err != nil {
		return coverageResult{}, "", err
	}
	actual, totalLine := calculateTotalCoverage(packageTotals, coverPackages)
	packageSummaries, err := summarizePackageCoverageFromReport(packageSummaryReport, coverPackages)
	if err != nil {
		return coverageResult{}, "", err
	}
	insufficientCoveragePackages := findInsufficientCoveragePackages(packageSummaries, minCoverage, baselinePackages)
	zeroCoveragePackages := findZeroCoveragePackagesFromSummaries(packageSummaries, baselinePackages)

	return coverageResult{
		actual:                       actual,
		insufficientCoveragePackages: insufficientCoveragePackages,
		packageSummaries:             packageSummaries,
		zeroCoveragePackages:         zeroCoveragePackages,
	}, totalLine, nil
}

func readPackageCoverageBaseline(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go coverage package baseline: %w", err)
	}

	packages := make(map[string]struct{})
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		packages[line] = struct{}{}
	}
	return packages, nil
}

func readCoverageProfileTotals(profilePath string, repoRoot string) (map[string]packageCoverageTotals, error) {
	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read go coverage profile: %w", err)
	}
	packageTotals, err := parseCoverageProfile(profileData, repoRoot)
	if err != nil {
		return nil, err
	}
	return packageTotals, nil
}

func calculateTotalCoverage(packageTotals map[string]packageCoverageTotals, coverPackages []string) (float64, string) {
	totalCovered, totalStatements := summedCoverageTotals(packageTotals, coverPackages)
	if totalStatements == 0 {
		return 0, "total: (statements) 0.0%"
	}
	actual := float64(totalCovered) * 100 / float64(totalStatements)
	return actual, fmt.Sprintf("total: (statements) %.1f%%", actual)
}

func summedCoverageTotals(packageTotals map[string]packageCoverageTotals, coverPackages []string) (int, int) {
	selectedPackages := selectedCoveragePackages(coverPackages)
	totalCovered := 0
	totalStatements := 0
	for _, coverPackage := range selectedPackages {
		totals := packageTotals[coverPackage]
		totalCovered += totals.coveredStatements
		totalStatements += totals.totalStatements
	}
	return totalCovered, totalStatements
}

func selectedCoveragePackages(coverPackages []string) []string {
	seen := make(map[string]struct{}, len(coverPackages))
	selected := make([]string, 0, len(coverPackages))
	for _, coverPackage := range coverPackages {
		if !isBackendCoveragePackage(coverPackage) {
			continue
		}
		if _, ok := seen[coverPackage]; ok {
			continue
		}
		seen[coverPackage] = struct{}{}
		selected = append(selected, coverPackage)
	}
	slices.Sort(selected)
	return selected
}

func summarizePackageCoverageFromReport(report string, coverPackages []string) ([]packageCoverageSummary, error) {
	selectedPackages := selectedCoveragePackages(coverPackages)
	reportCoverage, err := parsePackageCoverageSummariesFromReport(report)
	if err != nil {
		return nil, err
	}
	summaries := make([]packageCoverageSummary, 0, len(selectedPackages))
	for _, coverPackage := range selectedPackages {
		coverage := reportCoverage[coverPackage]
		summaries = append(summaries, packageCoverageSummary{
			importPath: coverPackage,
			coverage:   coverage,
		})
	}
	return summaries, nil
}

func findInsufficientCoveragePackages(summaries []packageCoverageSummary, minCoverage float64, baselinePackages map[string]struct{}) []packageCoverageSummary {
	packages := make([]packageCoverageSummary, 0)
	for _, summary := range summaries {
		if summary.coverage >= minCoverage {
			continue
		}
		if _, ok := baselinePackages[summary.importPath]; ok {
			continue
		}
		packages = append(packages, summary)
	}
	return packages
}

func parsePackageCoverageSummariesFromReport(report string) (map[string]float64, error) {
	packages := make(map[string]float64)
	for _, rawLine := range strings.Split(report, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "total:") {
			continue
		}
		matches := packageCoveragePattern.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		coveragePercent, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse go coverage package percentage %q: %w", matches[2], err)
		}
		if coveragePercent == 0 {
			packages[matches[1]] = 0
			continue
		}
		packages[matches[1]] = coveragePercent
	}
	return packages, nil
}

func findZeroCoveragePackagesFromSummaries(summaries []packageCoverageSummary, baselinePackages map[string]struct{}) []string {
	zeroCoveragePackages := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary.coverage != 0 {
			continue
		}
		if _, ok := baselinePackages[summary.importPath]; ok {
			continue
		}
		zeroCoveragePackages = append(zeroCoveragePackages, summary.importPath)
	}
	slices.Sort(zeroCoveragePackages)
	return zeroCoveragePackages
}

func parseCoverageProfile(profileData []byte, repoRoot string) (map[string]packageCoverageTotals, error) {
	lines := strings.Split(strings.TrimSpace(string(profileData)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, errors.New("parse go coverage profile: empty profile")
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "mode:") {
		return nil, errors.New("parse go coverage profile: missing mode header")
	}

	coverageBlocks := make(map[string]coverageBlock)
	for index, rawLine := range lines[1:] {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("parse go coverage profile: malformed line %d", index+2)
		}

		filePathWithRanges := fields[0]
		rangeSeparator := strings.LastIndex(filePathWithRanges, ":")
		if rangeSeparator < 0 {
			return nil, fmt.Errorf("parse go coverage profile: malformed file range on line %d", index+2)
		}

		statementCount, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse go coverage profile statements on line %d: %w", index+2, err)
		}
		executionCount, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("parse go coverage profile execution count on line %d: %w", index+2, err)
		}

		importPath, err := coverageImportPath(filePathWithRanges[:rangeSeparator], repoRoot)
		if err != nil {
			return nil, fmt.Errorf("parse go coverage profile import path on line %d: %w", index+2, err)
		}
		if executionCount > 0 {
			executionCount = 1
		}

		filePath := filePathWithRanges[:rangeSeparator]
		rangeSpec := filePathWithRanges[rangeSeparator+1:]
		canonicalFilePath, err := coverageCanonicalFilePath(filePath, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("parse go coverage profile canonical path on line %d: %w", index+2, err)
		}
		blockKey := fmt.Sprintf("%s:%s %d", canonicalFilePath, rangeSpec, statementCount)
		block := coverageBlocks[blockKey]
		block.importPath = importPath
		block.statementCount = statementCount
		if executionCount > block.executionCount {
			block.executionCount = executionCount
		}
		coverageBlocks[blockKey] = block
	}

	packageTotals := make(map[string]packageCoverageTotals)
	for _, block := range coverageBlocks {
		totals := packageTotals[block.importPath]
		totals.totalStatements += block.statementCount
		if block.executionCount > 0 {
			totals.coveredStatements += block.statementCount
		}
		packageTotals[block.importPath] = totals
	}

	return packageTotals, nil
}

func coverageCanonicalFilePath(filePath string, repoRoot string) (string, error) {
	normalizedPath := filepath.ToSlash(strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/"))
	if normalizedPath == "" {
		return "", errors.New("empty file path")
	}
	if normalizedPath == modulePath {
		return "", fmt.Errorf("profile path %q does not include a package directory", filePath)
	}

	switch {
	case strings.HasPrefix(normalizedPath, modulePath+"/"):
		return normalizedPath, nil
	case strings.HasPrefix(normalizedPath, "./"):
		normalizedPath = strings.TrimPrefix(normalizedPath, "./")
	case filepath.IsAbs(filePath):
		relativePath, err := filepath.Rel(repoRoot, filePath)
		if err != nil {
			return "", fmt.Errorf("resolve profile path relative to repository root: %w", err)
		}
		normalizedPath = filepath.ToSlash(relativePath)
	}

	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	if strings.HasPrefix(normalizedPath, "../") || normalizedPath == ".." {
		return "", fmt.Errorf("profile path %q escapes repository root", filePath)
	}

	importDir := path.Dir(normalizedPath)
	if importDir == "." || importDir == "" {
		return "", fmt.Errorf("profile path %q does not include a package directory", filePath)
	}
	return modulePath + "/" + normalizedPath, nil
}

func coverageImportPath(filePath string, repoRoot string) (string, error) {
	canonicalFilePath, err := coverageCanonicalFilePath(filePath, repoRoot)
	if err != nil {
		return "", err
	}
	return path.Dir(canonicalFilePath), nil
}

func formatZeroCoverageFailure(packages []string) string {
	return fmt.Sprintf(
		"go coverage found backend-owned packages with 0%% statement coverage: %s",
		strings.Join(packages, ", "),
	)
}

func formatInsufficientCoverageFailure(packages []packageCoverageSummary, minCoverage float64) string {
	formatted := make([]string, 0, len(packages))
	for _, summary := range packages {
		formatted = append(formatted, fmt.Sprintf("%s (%.1f%%)", summary.importPath, summary.coverage))
	}
	return fmt.Sprintf(
		"go coverage found non-baselined backend packages below %.1f%% statement coverage: %s",
		minCoverage,
		strings.Join(formatted, ", "),
	)
}

func failf(format string, args ...any) {
	fmt.Fprintf(stderrWriter, format, args...)
	exitFunc(1)
}
