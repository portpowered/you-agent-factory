package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

var (
	totalCoveragePattern       = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)
	coveragePackageListPattern = regexp.MustCompile(`(?m)(coverage:\s+[0-9.]+% of statements)\s+in\s+.+$`)
)

type commandInvocation struct {
	name         string
	args         []string
	env          []string
	dir          string
	stdoutWriter io.Writer
	stderrWriter io.Writer
}

type commandRunnerFunc func(commandInvocation) (string, string, error)

const modulePath = "github.com/portpowered/infinite-you"
const defaultPackageCoverageBaselinePath = "docs/internal/baselines/go-coverage-package-baseline.txt"
const defaultFunctionalPackageCoverageBaselinePath = "docs/internal/baselines/go-functional-coverage-package-baseline.txt"
const defaultPackageCoverageMin = 80.0
const defaultPackageFloorEpsilon = 0.25
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
		if invocation.stdoutWriter == nil {
			cmd.Stdout = &stdout
		} else {
			cmd.Stdout = io.MultiWriter(&stdout, invocation.stdoutWriter)
		}
		if invocation.stderrWriter == nil {
			cmd.Stderr = &stderr
		} else {
			cmd.Stderr = io.MultiWriter(&stderr, invocation.stderrWriter)
		}
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	covermode            string
	coverpkg             string
	functionalQuarantine string
	jobs                 int
	generateManifest     string
	updateManifest       string
	updateProfiles       string
	packageManifest      string
	varianceProfiles     string
	varianceOutput       string
	varianceCommit       string
	varianceJobs         int
	varianceAnnotations  string
	jsonOutput           string
	timingOutput         string
	min                  float64
	packageBaseline      string
	packageMin           float64
	packageFloorEpsilon  float64
	packages             string
	profile              string
	short                bool
	suite                string
	stream               bool
	timeout              time.Duration
	totalOnly            bool
}

type coverageResult struct {
	actual                       float64
	insufficientCoveragePackages []packageCoverageSummary
	coverageBlocks               map[string]coverageBlock
	packageTotals                map[string]packageCoverageTotals
	packageSummaries             []packageCoverageSummary
	packageGates                 map[string]packageCoverageGate
	zeroCoveragePackages         []string
	packageMinimumFailures       []string
	packageMinimumWarnings       []string
	unmeasuredPackageDiagnostics []string
}

type packageCoverageTotals struct {
	coveredStatements int
	totalStatements   int
}

type packageCoverageSummary struct {
	importPath string
	coverage   float64
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
	if cfg.varianceProfiles != "" || cfg.varianceOutput != "" {
		return executeVarianceReport(cfg)
	}
	if strings.TrimSpace(cfg.updateProfiles) != "" {
		return executeSampledManifestUpdate(cfg)
	}
	result, err := run(cfg)
	if err != nil {
		writeCoverageTestFailureWarning(err)
		var validationErr *coverageManifestValidationError
		if errors.As(err, &validationErr) {
			writePackageCoverageSummaries(result.packageSummaries)
		}
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

	writePackageCoverageSummaries(result.packageSummaries)
	if cfg.generateManifest != "" {
		if err := createCoverageManifest(cfg.generateManifest, cfg.suite, result.packageTotals, packageImportPaths(result.packageSummaries)); err != nil {
			return err
		}
		fmt.Fprintf(stdoutWriter, "Created %s coverage manifest at %s.\n", cfg.suite, cfg.generateManifest)
	}
	if err := writeCoverageSummaryJSON(cfg.jsonOutput, result); err != nil {
		return err
	}
	writeCoverageDiagnostics(result)

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
	flag.StringVar(&cfg.functionalQuarantine, "functional-quarantine", "", "strict functional quarantine JSON manifest; discovers and subtracts its package/test selectors")
	flag.IntVar(&cfg.jobs, "jobs", 0, "maximum concurrent go test packages; defaults to 2")
	flag.StringVar(&cfg.generateManifest, "generate-manifest", "", "create a deterministic package-minimum manifest from this lane's coverage profile")
	flag.StringVar(&cfg.updateManifest, "update-manifest", "", "update an existing package-minimum manifest from a complete compatible profile sample set")
	flag.StringVar(&cfg.updateProfiles, "update-profiles", "", "comma-separated complete coverage profiles for -update-manifest; requires at least five compatible profiles")
	flag.StringVar(&cfg.packageManifest, "package-manifest", "", "enforce the active lane's checked-in package-minimum manifest")
	flag.StringVar(&cfg.varianceProfiles, "variance-profiles", "", "comma-separated functional coverage profiles to aggregate into a variance report")
	flag.StringVar(&cfg.varianceOutput, "variance-output", "", "write a deterministic functional coverage variance report to this path")
	flag.StringVar(&cfg.varianceCommit, "variance-commit", "", "full unchanged commit SHA named by a functional coverage variance report")
	flag.IntVar(&cfg.varianceJobs, "variance-jobs", defaultCoverageJobs, "package-concurrency setting used to capture the profiles named by a variance report")
	flag.StringVar(&cfg.varianceAnnotations, "variance-annotations", "", "optional validated JSON annotations to append to a variance report")
	flag.StringVar(&cfg.jsonOutput, "json-output", "", "optional path for a deterministic machine-readable coverage summary JSON document")
	flag.StringVar(&cfg.timingOutput, "timing-output", "", "optional path for a deterministic machine-readable functional package timing summary JSON document, captured from the same go test run")
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packageBaseline, "package-baseline", "", "newline-delimited list of backend packages temporarily exempt from the per-package minimum coverage gate; defaults by suite")
	flag.Float64Var(&cfg.packageMin, "package-min", defaultPackageCoverageMin, "minimum statement coverage required for each non-baselined backend package")
	flag.Float64Var(&cfg.packageFloorEpsilon, "package-floor-epsilon", defaultPackageFloorEpsilon, "allowed manifest package-floor drift in percentage points; only applies with -package-manifest")
	flag.StringVar(&cfg.packages, "packages", "", "space-separated go test package patterns; overrides -suite package discovery")
	flag.StringVar(&cfg.profile, "profile", "", "coverage profile output path; defaults to a temp file")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.StringVar(&cfg.suite, "suite", "unit", "test suite to execute when -packages is empty: unit or functional")
	flag.BoolVar(&cfg.stream, "stream", false, "stream coverage-test child stdout and stderr to their output sinks while running")
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
	if cfg.packageFloorEpsilon < 0 || math.IsNaN(cfg.packageFloorEpsilon) || math.IsInf(cfg.packageFloorEpsilon, 0) {
		return fmt.Errorf("configure go coverage: -package-floor-epsilon must be a finite non-negative percentage-point value (got %v); set it to 0 or greater", cfg.packageFloorEpsilon)
	}
	if strings.TrimSpace(cfg.updateManifest) != "" && strings.TrimSpace(cfg.updateProfiles) == "" {
		return errors.New("configure go coverage manifest update: -update-manifest requires -update-profiles with at least five compatible profiles")
	}
	if strings.TrimSpace(cfg.functionalQuarantine) != "" && cfg.suite != "functional" {
		return fmt.Errorf("configure functional quarantine: -functional-quarantine requires -suite functional (got %q)", cfg.suite)
	}
	if strings.TrimSpace(cfg.updateProfiles) != "" && strings.TrimSpace(cfg.updateManifest) == "" {
		return errors.New("configure go coverage manifest update: -update-profiles requires -update-manifest")
	}
	varianceRequested := strings.TrimSpace(cfg.varianceProfiles) != "" || strings.TrimSpace(cfg.varianceOutput) != ""
	if strings.TrimSpace(cfg.varianceAnnotations) != "" && !varianceRequested {
		return errors.New("configure coverage variance: -variance-annotations requires -variance-profiles and -variance-output")
	}
	if varianceRequested {
		if strings.TrimSpace(cfg.varianceProfiles) == "" || strings.TrimSpace(cfg.varianceOutput) == "" {
			return errors.New("configure coverage variance: -variance-profiles and -variance-output must be provided together")
		}
		if cfg.suite != "functional" {
			return fmt.Errorf("configure coverage variance: -suite must be functional (got %q)", cfg.suite)
		}
		if strings.TrimSpace(cfg.generateManifest) != "" || strings.TrimSpace(cfg.updateManifest) != "" || strings.TrimSpace(cfg.updateProfiles) != "" {
			return errors.New("configure coverage variance: do not combine variance reporting with manifest generation or update")
		}
		if strings.TrimSpace(cfg.varianceCommit) == "" {
			return errors.New("configure coverage variance: -variance-commit must name the unchanged commit used for every profile")
		}
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
	return runForOS(cfg, runtime.GOOS)
}

func runForOS(cfg config, targetOS string) (result coverageResult, runErr error) {
	profilePath, cleanup, err := prepareCoverageProfile(cfg.profile)
	if err != nil {
		return coverageResult{}, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("remove temporary coverage profile: %w", cleanupErr))
		}
	}()
	return runCoverageProfile(cfg, targetOS, profilePath)
}

func prepareCoverageProfile(profilePath string) (string, func() error, error) {
	if profilePath != "" {
		return profilePath, func() error { return nil }, nil
	}
	file, err := os.CreateTemp("", "go-coverage-*.out")
	if err != nil {
		return "", nil, fmt.Errorf("create temp coverage profile: %w", err)
	}
	profilePath = file.Name()
	cleanup := func() error { return os.Remove(profilePath) }
	if err := file.Close(); err != nil {
		return "", nil, errors.Join(
			fmt.Errorf("close temp coverage profile: %w", err),
			cleanup(),
		)
	}
	return profilePath, cleanup, nil
}

func runCoverageProfile(cfg config, targetOS string, profilePath string) (coverageResult, error) {
	coverPackages, testPackages, err := resolveCoverageLane(cfg)
	if err != nil {
		return coverageResult{}, err
	}
	repoRoot, err := repoRootDir()
	if err != nil {
		return coverageResult{}, err
	}
	var functionalSelection *functionalCoverageSelection
	if strings.TrimSpace(cfg.functionalQuarantine) != "" {
		selection, selectedPackages, selectionErr := prepareFunctionalCoverageRun(cfg, testPackages, targetOS, repoRoot)
		if selectionErr != nil {
			return coverageResult{}, selectionErr
		}
		functionalSelection = &selection
		testPackages = selectedPackages
	}
	coverPackageArgument := strings.Join(coverPackages, ",")
	if targetOS == "windows" && strings.TrimSpace(cfg.coverpkg) == "" {
		// A fully expanded backend package list exceeds Windows' command-line
		// limit. A package pattern keeps the invocation to one logical coverage
		// pass; the resolved list above remains authoritative for profile
		// filtering, reporting, and package gates.
		coverPackageArgument = modulePath + "/pkg/..."
	}
	coverageTestArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", coverPackageArgument),
		fmt.Sprintf("-p=%d", cfg.testJobs(targetOS)),
		// Coverage is an authoritative measurement, so every package must run
		// even when a prior non-instrumented invocation is cached.
		"-count=1",
	}
	if cfg.short {
		coverageTestArgs = append(coverageTestArgs, "-short")
	}
	coverageTestArgs = append(coverageTestArgs,
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)

	testPackageArgs := compactUnitTestPackageArgs(cfg, testPackages, targetOS)
	var runErr error
	if functionalSelection == nil {
		runErr = runGoTestCoverageLane(cfg, coverageTestArgs, testPackages, profilePath, repoRoot, coverPackages, targetOS, "run go test coverage lane", testPackageArgs)
	} else {
		runErr = runGoTestCoverageLaneWithSelection(cfg, coverageTestArgs, testPackages, profilePath, repoRoot, coverPackages, targetOS, "run go test coverage lane", *functionalSelection)
	}
	if runErr != nil {
		return coverageResult{}, runErr
	}
	if err := canonicalizeCoverageProfile(profilePath, repoRoot, coverPackages); err != nil {
		return coverageResult{}, err
	}

	legacyPackageGateEnabled := !cfg.totalOnly && cfg.generateManifest == "" && cfg.updateManifest == "" && strings.TrimSpace(cfg.packageManifest) == ""
	baselinePackages := map[string]struct{}{}
	if legacyPackageGateEnabled {
		baselinePackages, err = packageCoverageBaselinePackages(cfg, repoRoot)
		if err != nil {
			return coverageResult{}, err
		}
	}

	result, totalLine, err := evaluateCoverage("", "", profilePath, repoRoot, coverPackages, cfg.packageCoverageMin(), baselinePackages, legacyPackageGateEnabled)
	if err != nil {
		return coverageResult{}, err
	}
	fmt.Fprintln(stdoutWriter, totalLine)
	if !cfg.totalOnly && strings.TrimSpace(cfg.packageManifest) != "" {
		manifestPath := cfg.packageManifest
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(repoRoot, manifestPath)
		}
		manifest, err := readCoverageManifestFileWithTotals(manifestPath, cfg.suite, packageImportPaths(result.packageSummaries), result.packageTotals)
		if err != nil {
			var validationErr *coverageManifestValidationError
			if errors.As(err, &validationErr) {
				return result, err
			}
			return coverageResult{}, err
		}
		result.packageMinimumFailures, result.packageMinimumWarnings = checkCoverageManifestWithEpsilonAndBlocks(manifest, result.packageTotals, cfg.packageManifest, cfg.packageFloorEpsilon, result.coverageBlocks)
		result.unmeasuredPackageDiagnostics = formatUnmeasuredCoverageManifestDiagnostics(manifest, result.packageTotals)
		result.packageGates = packageGatesFromManifest(manifest)
	} else if legacyPackageGateEnabled {
		result.packageGates = packageGatesFromLegacyMin(result.packageSummaries, cfg.packageCoverageMin(), baselinePackages)
	}
	return result, nil
}

func writePackageCoverageSummaries(summaries []packageCoverageSummary) {
	for _, summary := range summaries {
		fmt.Fprintf(stdoutWriter, "%s\tcoverage: %.1f%% of statements\n", summary.importPath, summary.coverage)
	}
}

func (cfg config) testJobs(targetOS string) int {
	if cfg.jobs > 0 {
		return cfg.jobs
	}
	if targetOS == "windows" && (cfg.suite == "" || cfg.suite == "unit") {
		// Full unit coverage instrumentation exceeds the Windows host's stable
		// memory boundary when go test builds two instrumented packages at once.
		return 1
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
		return listGoPackages(functionalTestPatterns, isFunctionalTestPackage, false)
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

func compactUnitTestPackageArgs(cfg config, testPackages []string, targetOS string) []string {
	// The compact patterns are safe only for the default unit package universe:
	// custom package lists and functional packages retain their existing args.
	if targetOS != "windows" || (cfg.suite != "" && cfg.suite != "unit") || strings.TrimSpace(cfg.packages) != "" {
		return nil
	}
	allPackages, err := listGoPackages([]string{"./pkg/..."}, func(string) bool { return true }, false)
	if err != nil {
		return nil
	}
	patterns, err := compactGoPackagePatterns(allPackages, testPackages, modulePath+"/pkg")
	if err != nil {
		return nil
	}
	return patterns
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
	coverageBlocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		return coverageResult{}, "", err
	}
	packageTotals := coverageTotals(coverageBlocks)
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
		coverageBlocks:               coverageBlocks,
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
	coverageBlocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageTotals(coverageBlocks), nil
}

func readCoverageProfileBlocks(profilePath string, repoRoot string) (map[string]coverageBlock, error) {
	profile, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read go coverage profile: %w", err)
	}
	defer profile.Close()

	_, coverageBlocks, err := scanCoverageProfile(profile, repoRoot)
	if err != nil {
		return nil, err
	}
	return coverageBlocks, nil
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
