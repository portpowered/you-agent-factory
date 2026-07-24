package main

import (
	"bufio"
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
	"sync"
	"time"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

var (
	totalCoveragePattern       = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)
	coveragePackageListPattern = regexp.MustCompile(`(?m)(coverage:\s+[0-9.]+% of statements)\s+in\s+.+$`)
)

type commandInvocation struct {
	name string
	args []string
	env  []string
	dir  string
}

type commandRunnerFunc func(commandInvocation) (string, string, error)

const modulePath = "github.com/portpowered/infinite-you"
const defaultPackageCoverageBaselinePath = "docs/internal/baselines/go-coverage-package-baseline.txt"
const defaultFunctionalPackageCoverageBaselinePath = "docs/internal/baselines/go-functional-coverage-package-baseline.txt"
const defaultPackageCoverageMin = 80.0
const defaultCoverageJobs = 2

var (
	defaultCoveragePatterns                   = []string{"./pkg/..."}
	unitTestPatterns                          = []string{"./pkg/..."}
	functionalTestPatterns                    = []string{testlanes.FunctionalPackagePattern}
	execCommand                               = exec.Command
	commandRunner           commandRunnerFunc = func(invocation commandInvocation) (string, string, error) {
		cmd := execCommand(invocation.name, invocation.args...)
		if invocation.env != nil {
			cmd.Env = invocation.env
		}
		if invocation.dir != "" {
			cmd.Dir = invocation.dir
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	covermode        string
	coverpkg         string
	jobs             int
	generateManifest string
	updateManifest   string
	packageManifest  string
	jsonOutput       string
	min              float64
	packageBaseline  string
	packageMin       float64
	packages         string
	profile          string
	short            bool
	suite            string
	timeout          time.Duration
	totalOnly        bool
}

type coverageResult struct {
	actual                       float64
	insufficientCoveragePackages []packageCoverageSummary
	packageTotals                map[string]packageCoverageTotals
	packageSummaries             []packageCoverageSummary
	zeroCoveragePackages         []string
	packageMinimumFailures       []string
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
	canonicalPath  string
	importPath     string
	rangeSpec      string
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
	if err := validateConfig(cfg); err != nil {
		return err
	}
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
	failures = append(failures, result.packageMinimumFailures...)

	for _, summary := range result.packageSummaries {
		fmt.Fprintf(stdoutWriter, "%s\tcoverage: %.1f%% of statements\n", summary.importPath, summary.coverage)
	}
	if cfg.generateManifest != "" {
		if err := createCoverageManifest(cfg.generateManifest, cfg.suite, result.packageTotals, packageImportPaths(result.packageSummaries)); err != nil {
			return err
		}
		fmt.Fprintf(stdoutWriter, "Created %s coverage manifest at %s.\n", cfg.suite, cfg.generateManifest)
	}
	if cfg.updateManifest != "" {
		updates, err := updateCoverageManifestFile(cfg.updateManifest, cfg.suite, result.packageTotals, packageImportPaths(result.packageSummaries))
		for _, update := range updates {
			fmt.Fprintln(stdoutWriter, update.String())
		}
		if err != nil {
			return err
		}
	}
	if err := writeCoverageSummaryJSON(cfg.jsonOutput, result); err != nil {
		return err
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
	flag.IntVar(&cfg.jobs, "jobs", 0, "number of isolated coverage shards; defaults to 2")
	flag.StringVar(&cfg.generateManifest, "generate-manifest", "", "create a deterministic package-minimum manifest from this lane's coverage profile")
	flag.StringVar(&cfg.updateManifest, "update-manifest", "", "monotonically add or raise floors in an existing package-minimum manifest")
	flag.StringVar(&cfg.packageManifest, "package-manifest", "", "enforce the active lane's checked-in package-minimum manifest")
	flag.StringVar(&cfg.jsonOutput, "json-output", "", "optional path for a deterministic machine-readable coverage summary JSON document")
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packageBaseline, "package-baseline", "", "newline-delimited list of backend packages temporarily exempt from the per-package minimum coverage gate; defaults by suite")
	flag.Float64Var(&cfg.packageMin, "package-min", defaultPackageCoverageMin, "minimum statement coverage required for each non-baselined backend package")
	flag.StringVar(&cfg.packages, "packages", "", "space-separated go test package patterns; overrides -suite package discovery")
	flag.StringVar(&cfg.profile, "profile", "", "coverage profile output path; defaults to a temp file")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.StringVar(&cfg.suite, "suite", "unit", "test suite to execute when -packages is empty: unit or functional")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.BoolVar(&cfg.totalOnly, "total-only", false, "disable package-local coverage gates while retaining per-package reporting")
	flag.Parse()
	return cfg
}

func validateConfig(cfg config) error {
	manifestOperations := 0
	for _, value := range []string{cfg.generateManifest, cfg.updateManifest, cfg.packageManifest} {
		if strings.TrimSpace(value) != "" {
			manifestOperations++
		}
	}
	if manifestOperations > 1 {
		return errors.New("configure go coverage: choose only one of -generate-manifest, -update-manifest, or -package-manifest")
	}
	return nil
}

func (cfg config) packageCoverageBaselinePath() string {
	if strings.TrimSpace(cfg.packageBaseline) == "" {
		if cfg.suite == "functional" {
			return defaultFunctionalPackageCoverageBaselinePath
		}
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
	repoRoot, err := repoRootDir()
	if err != nil {
		return coverageResult{}, err
	}
	mergedTestArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", strings.Join(coverPackages, ",")),
		"-p=1",
	}
	if cfg.short {
		mergedTestArgs = append(mergedTestArgs, "-short")
	}
	mergedTestArgs = append(mergedTestArgs,
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)

	if err := runGoTestCoverageShards(mergedTestArgs, testPackages, cfg.testJobs(), profilePath, repoRoot, coverPackages); err != nil {
		return coverageResult{}, err
	}

	coverStdout, coverStderr, err := runCommand(commandInvocation{
		name: "go",
		args: []string{"tool", "cover", "-func", profilePath},
		env:  os.Environ(),
	})
	// Print for utility
	fmt.Println(formatCommandLine("go", "tool", "cover", "-func", profilePath))

	if err != nil {
		detail := strings.TrimSpace(coverStderr)
		if detail == "" {
			detail = strings.TrimSpace(coverStdout)
		}
		if detail != "" {
			return coverageResult{}, fmt.Errorf("summarize go coverage: %w\n%s", err, detail)
		}
		return coverageResult{}, fmt.Errorf("summarize go coverage: %w", err)
	}

	legacyPackageGateEnabled := !cfg.totalOnly && cfg.generateManifest == "" && cfg.updateManifest == "" && strings.TrimSpace(cfg.packageManifest) == ""
	baselinePackages := map[string]struct{}{}
	if legacyPackageGateEnabled {
		baselinePackages, err = packageCoverageBaselinePackages(cfg, repoRoot)
		if err != nil {
			return coverageResult{}, err
		}
	}

	result, totalLine, err := evaluateCoverage(coverStdout, "", profilePath, repoRoot, coverPackages, cfg.packageCoverageMin(), baselinePackages, legacyPackageGateEnabled)
	if err != nil {
		return coverageResult{}, err
	}
	if !cfg.totalOnly && strings.TrimSpace(cfg.packageManifest) != "" {
		manifestPath := cfg.packageManifest
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(repoRoot, manifestPath)
		}
		manifest, err := readCoverageManifestFile(manifestPath, cfg.suite, packageImportPaths(result.packageSummaries))
		if err != nil {
			return coverageResult{}, err
		}
		result.packageMinimumFailures = checkCoverageManifest(manifest, result.packageTotals, cfg.packageManifest)
	}
	fmt.Fprintln(stdoutWriter, totalLine)
	return result, nil
}

func (cfg config) testJobs() int {
	if cfg.jobs > 0 {
		return cfg.jobs
	}
	return defaultCoverageJobs
}

func packageCoverageBaselinePackages(cfg config, repoRoot string) (map[string]struct{}, error) {
	baselinePath := cfg.packageCoverageBaselinePath()
	if !filepath.IsAbs(baselinePath) {
		baselinePath = filepath.Join(repoRoot, baselinePath)
	}
	return readPackageCoverageBaseline(baselinePath)
}

func runGoTestCoverageLane(args []string, failurePrefix string) (string, string, error) {
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
	})
	if err != nil {
		detail := mergeGoTestFailureDetail(stderr, stdout)
		if detail != "" {
			return "", "", fmt.Errorf("%s: %w\n%s", failurePrefix, err, detail)
		}
		return "", "", fmt.Errorf("%s: %w", failurePrefix, err)
	}
	return stdout, stderr, nil
}

func runCommand(invocation commandInvocation) (string, string, error) {
	return commandRunner(invocation)
}

func formatCommandLine(name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func runGoTestCoverageShards(baseArgs []string, testPackages []string, jobs int, profilePath string, repoRoot string, coverPackages []string) error {
	if jobs < 1 {
		jobs = 1
	}
	if jobs > len(testPackages) {
		jobs = len(testPackages)
	}
	shardDir, err := os.MkdirTemp("", "go-coverage-shards-*")
	if err != nil {
		return fmt.Errorf("create go coverage shard directory: %w", err)
	}
	defer os.RemoveAll(shardDir)

	shards := make([][]string, jobs)
	for index, testPackage := range testPackages {
		shards[index%jobs] = append(shards[index%jobs], testPackage)
	}
	profiles := make([]string, jobs)
	errs := make([]error, jobs)
	var wait sync.WaitGroup
	for index, packages := range shards {
		profiles[index] = filepath.Join(shardDir, fmt.Sprintf("shard-%d.out", index+1))
		args := append(slices.Clone(baseArgs), fmt.Sprintf("-coverprofile=%s", profiles[index]))
		args = append(args, packages...)
		wait.Add(1)
		go func(index int, args []string) {
			defer wait.Done()
			_, _, errs[index] = runGoTestCoverageLane(args, fmt.Sprintf("run go test coverage shard %d/%d", index+1, jobs))
		}(index, args)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return mergeCoverageProfiles(profiles, profilePath, repoRoot, coverPackages)
}

func mergeGoTestFailureDetail(stderr string, stdout string) string {
	stderr = strings.TrimSpace(compactCoverageOutput(stderr))
	stdout = strings.TrimSpace(compactCoverageOutput(stdout))
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

func compactCoverageOutput(output string) string {
	return coveragePackageListPattern.ReplaceAllString(output, "$1")
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
	switch cfg.suite {
	case "", "unit":
		return listGoPackages(unitTestPatterns, isBackendCoveragePackage, false)
	case "functional":
		packages, err := listGoPackages(functionalTestPatterns, isFunctionalTestPackage, false)
		if err != nil {
			return nil, err
		}
		if err := testlanes.ValidateProviderFunctionalPackages(packages); err != nil {
			return nil, fmt.Errorf("resolve go coverage lane: %w", err)
		}
		return packages, nil
	default:
		return nil, fmt.Errorf("resolve go coverage lane: unsupported suite %q", cfg.suite)
	}
}

func listGoPackages(patterns []string, include func(string) bool, requireNonTestGoFiles bool) ([]string, error) {
	args := append([]string{"list", "-f", "{{.ImportPath}}	{{len .GoFiles}}"}, patterns...)
	rootDir, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  rootDir,
	})
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			return nil, fmt.Errorf("list go packages: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("list go packages: %w", err)
	}

	seen := make(map[string]struct{})
	var packages []string
	for _, line := range strings.Split(stdout, "\n") {
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
	fields := strings.Fields(strings.TrimSpace(line))
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
	case !strings.HasPrefix(importPath, modulePath+"/pkg/"):
		return false
	case importPath == modulePath+"/pkg/transports/http/generated":
		return false
	case importPath == modulePath+"/pkg/transports/http/client":
		return false
	case importPath == modulePath+"/pkg/transports/mcp/generated":
		return false
	case !testlanes.IsUnitPackage(importPath):
		return false
	default:
		return true
	}
}

func isFunctionalTestPackage(importPath string) bool {
	return testlanes.IsRunnableFunctionalPackage(importPath)
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

func evaluateCoverage(_ string, _ string, profilePath string, repoRoot string, coverPackages []string, minCoverage float64, baselinePackages map[string]struct{}, packageGate ...bool) (coverageResult, string, error) {
	packageTotals, err := readCoverageProfileTotals(profilePath, repoRoot)
	if err != nil {
		return coverageResult{}, "", err
	}
	actual, totalLine := calculateTotalCoverage(packageTotals, coverPackages)
	packageGateEnabled := len(packageGate) == 0 || packageGate[0]
	packageSummaries := summarizePackageCoverageFromTotals(packageTotals, coverPackages)
	var insufficientCoveragePackages []packageCoverageSummary
	if packageGateEnabled {
		insufficientCoveragePackages = findInsufficientCoveragePackages(packageSummaries, minCoverage, baselinePackages)
	}
	zeroCoveragePackages := findZeroCoveragePackagesFromSummaries(packageSummaries, baselinePackages)

	return coverageResult{
		actual:                       actual,
		insufficientCoveragePackages: insufficientCoveragePackages,
		packageTotals:                packageTotals,
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
	profile, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read go coverage profile: %w", err)
	}
	defer profile.Close()

	_, coverageBlocks, err := scanCoverageProfile(profile, repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageTotals(coverageBlocks), nil
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

func summarizePackageCoverageFromTotals(packageTotals map[string]packageCoverageTotals, coverPackages []string) []packageCoverageSummary {
	selectedPackages := selectedCoveragePackages(coverPackages)
	summaries := make([]packageCoverageSummary, 0, len(selectedPackages))
	for _, coverPackage := range selectedPackages {
		totals := packageTotals[coverPackage]
		coverage := 0.0
		if totals.totalStatements > 0 {
			coverage = float64(totals.coveredStatements) * 100 / float64(totals.totalStatements)
		}
		summaries = append(summaries, packageCoverageSummary{
			importPath: coverPackage,
			coverage:   coverage,
		})
	}
	return summaries
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
	_, coverageBlocks, err := scanCoverageProfile(bytes.NewReader(profileData), repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageTotals(coverageBlocks), nil
}

func scanCoverageProfile(profile io.Reader, repoRoot string) (string, map[string]coverageBlock, error) {
	scanner := bufio.NewScanner(profile)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) == "" {
		return "", nil, errors.New("parse go coverage profile: empty profile")
	}
	header := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(header, "mode:") {
		return "", nil, errors.New("parse go coverage profile: missing mode header")
	}

	coverageBlocks := make(map[string]coverageBlock)
	lineNumber := 1
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			return "", nil, fmt.Errorf("parse go coverage profile: malformed line %d", lineNumber)
		}

		filePathWithRanges := fields[0]
		rangeSeparator := strings.LastIndex(filePathWithRanges, ":")
		if rangeSeparator < 0 {
			return "", nil, fmt.Errorf("parse go coverage profile: malformed file range on line %d", lineNumber)
		}

		statementCount, err := strconv.Atoi(fields[1])
		if err != nil {
			return "", nil, fmt.Errorf("parse go coverage profile statements on line %d: %w", lineNumber, err)
		}
		executionCount, err := strconv.Atoi(fields[2])
		if err != nil {
			return "", nil, fmt.Errorf("parse go coverage profile execution count on line %d: %w", lineNumber, err)
		}

		importPath, err := coverageImportPath(filePathWithRanges[:rangeSeparator], repoRoot)
		if err != nil {
			return "", nil, fmt.Errorf("parse go coverage profile import path on line %d: %w", lineNumber, err)
		}
		if executionCount > 0 {
			executionCount = 1
		}

		filePath := filePathWithRanges[:rangeSeparator]
		rangeSpec := filePathWithRanges[rangeSeparator+1:]
		canonicalFilePath, err := coverageCanonicalFilePath(filePath, repoRoot)
		if err != nil {
			return "", nil, fmt.Errorf("parse go coverage profile canonical path on line %d: %w", lineNumber, err)
		}
		blockKey := canonicalFilePath + ":" + rangeSpec
		block := coverageBlocks[blockKey]
		if block.statementCount != 0 && block.statementCount != statementCount {
			return "", nil, fmt.Errorf("parse go coverage profile: source block %s has inconsistent statement counts %d and %d", blockKey, block.statementCount, statementCount)
		}
		block.canonicalPath = canonicalFilePath
		block.importPath = importPath
		block.rangeSpec = rangeSpec
		block.statementCount = statementCount
		if executionCount > block.executionCount {
			block.executionCount = executionCount
		}
		coverageBlocks[blockKey] = block
	}
	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("parse go coverage profile: %w", err)
	}
	return header, coverageBlocks, nil
}

func coverageTotals(coverageBlocks map[string]coverageBlock) map[string]packageCoverageTotals {
	packageTotals := make(map[string]packageCoverageTotals)
	for _, block := range coverageBlocks {
		totals := packageTotals[block.importPath]
		totals.totalStatements += block.statementCount
		if block.executionCount > 0 {
			totals.coveredStatements += block.statementCount
		}
		packageTotals[block.importPath] = totals
	}

	return packageTotals
}

func canonicalizeCoverageProfile(profilePath string, repoRoot string, coverPackages []string) error {
	return mergeCoverageProfiles([]string{profilePath}, profilePath, repoRoot, coverPackages)
}

func mergeCoverageProfiles(profilePaths []string, outputPath string, repoRoot string, coverPackages []string) error {
	coverageBlocks := make(map[string]coverageBlock)
	header := ""
	for _, profilePath := range profilePaths {
		profile, err := os.Open(profilePath)
		if err != nil {
			return fmt.Errorf("read go coverage profile: %w", err)
		}
		profileHeader, profileBlocks, scanErr := scanCoverageProfile(profile, repoRoot)
		closeErr := profile.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return fmt.Errorf("close go coverage profile: %w", closeErr)
		}
		if header == "" {
			header = profileHeader
		} else if header != profileHeader {
			return fmt.Errorf("merge go coverage profiles: mode headers differ: %q and %q", header, profileHeader)
		}
		for key, block := range profileBlocks {
			merged := coverageBlocks[key]
			if merged.statementCount != 0 && merged.statementCount != block.statementCount {
				return fmt.Errorf("merge go coverage profiles: source block %s has inconsistent statement counts %d and %d", key, merged.statementCount, block.statementCount)
			}
			if block.executionCount > merged.executionCount {
				merged.executionCount = block.executionCount
			}
			merged.canonicalPath = block.canonicalPath
			merged.importPath = block.importPath
			merged.rangeSpec = block.rangeSpec
			merged.statementCount = block.statementCount
			coverageBlocks[key] = merged
		}
	}

	selected := make(map[string]struct{}, len(coverPackages))
	for _, importPath := range selectedCoveragePackages(coverPackages) {
		selected[importPath] = struct{}{}
	}
	keys := make([]string, 0, len(coverageBlocks))
	for key, block := range coverageBlocks {
		if _, ok := selected[block.importPath]; ok {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("rewrite canonical go coverage profile: %w", err)
	}
	writer := bufio.NewWriter(output)
	_, writeErr := fmt.Fprintln(writer, header)
	for _, key := range keys {
		if writeErr != nil {
			break
		}
		block := coverageBlocks[key]
		_, writeErr = fmt.Fprintf(writer, "%s:%s %d %d\n", block.canonicalPath, block.rangeSpec, block.statementCount, block.executionCount)
	}
	if writeErr == nil {
		writeErr = writer.Flush()
	}
	closeErr := output.Close()
	if writeErr != nil {
		return fmt.Errorf("rewrite canonical go coverage profile: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close canonical go coverage profile: %w", closeErr)
	}
	return nil
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
