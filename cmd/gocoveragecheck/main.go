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

const (
	coverageFloorPolicyBlocking = "blocking"
	coverageFloorPolicyAdvisory = "advisory"
)

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
	covermode                      string
	coverpkg                       string
	functionalQuarantine           string
	validateFunctionalQuarantine   bool
	jobs                           int
	generateManifest               string
	updateManifest                 string
	updateProfiles                 string
	packageManifest                string
	varianceProfiles               string
	varianceOutput                 string
	varianceCommit                 string
	varianceJobs                   int
	varianceAnnotations            string
	jsonOutput                     string
	timingOutput                   string
	coverageBuildDiagnosticsOutput string
	min                            float64
	packageBaseline                string
	packageMin                     float64
	packageFloorEpsilon            float64
	packageFloorPolicy             string
	detailedDiagnostics            bool
	packages                       string
	profile                        string
	short                          bool
	suite                          string
	stream                         bool
	timeout                        time.Duration
	totalOnly                      bool
	phaseTiming                    *coveragePhaseTimer
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
	manifestCompletenessWarnings []string
	unmeasuredPackageDiagnostics []string
	packageFloorPolicy           string
	detailedDiagnostics          bool
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
	if cfg.validateFunctionalQuarantine {
		return executeFunctionalQuarantineValidation(cfg)
	}
	if cfg.varianceProfiles != "" || cfg.varianceOutput != "" {
		return executeVarianceReport(cfg)
	}
	if strings.TrimSpace(cfg.updateProfiles) != "" {
		return executeSampledManifestUpdate(cfg)
	}
	if cfg.packageFloorPolicyIsAdvisory() {
		writeAdvisoryFloorPolicyBanner()
	}
	if cfg.phaseTiming == nil && unitCoveragePhaseTimingEnabled(cfg) {
		cfg.phaseTiming = newCoveragePhaseTimer(stdoutWriter)
		defer cfg.phaseTiming.emit()
	}
	result, err := run(cfg)
	if err != nil {
		result.packageFloorPolicy = cfg.packageFloorPolicyValue()
		result.detailedDiagnostics = cfg.detailedDiagnostics
		writeCoverageTestFailureWarning(err)
		var validationErr *coverageManifestValidationError
		if errors.As(err, &validationErr) {
			// A malformed manifest aborts before the coverage-summary JSON is
			// written, so keep the complete per-package listing: it is the
			// only surviving copy of the measurement on this abort path.
			writePackageCoverageSummaries(result.packageSummaries)
		}
		return err
	}
	result.packageFloorPolicy = cfg.packageFloorPolicyValue()
	result.detailedDiagnostics = cfg.detailedDiagnostics

	var failures []string
	if result.actual < cfg.min {
		failures = append(failures, fmt.Sprintf("go coverage %.1f%% is below minimum %.1f%%", result.actual, cfg.min))
	}
	if len(result.insufficientCoveragePackages) > 0 {
		insufficientFailure := formatInsufficientCoverageFailure(result.insufficientCoveragePackages, cfg.packageCoverageMin())
		if cfg.packageFloorPolicyIsAdvisory() {
			result.packageMinimumWarnings = append(result.packageMinimumWarnings, insufficientFailure)
		} else {
			failures = append(failures, insufficientFailure)
		}
	}
	failures = append(failures, result.packageMinimumFailures...)

	writeCoverageLaneReport(cfg, result, failures)
	if cfg.generateManifest != "" {
		if err := createCoverageManifest(cfg.generateManifest, cfg.suite, result.packageTotals, packageImportPaths(result.packageSummaries)); err != nil {
			cfg.finishCoveragePhase(coveragePhaseManifest, coveragePhaseStatusError)
			return err
		}
		fmt.Fprintf(stdoutWriter, "Created %s coverage manifest at %s.\n", cfg.suite, cfg.generateManifest)
	}
	if err := writeCoverageSummaryJSON(cfg.jsonOutput, result); err != nil {
		cfg.finishCoveragePhase(coveragePhaseManifest, coveragePhaseStatusError)
		return err
	}
	writeCoverageDiagnostics(result)
	cfg.finishCoveragePhase(coveragePhaseManifest, coveragePhaseStatusComplete)

	if len(failures) > 0 {
		return errors.New(strings.Join(coverageDiagnosticsForOutput(cfg.detailedDiagnostics, failures), "\n"))
	}
	fmt.Fprintf(stdoutWriter, "Go coverage %.1f%% meets minimum %.1f%%.\n", result.actual, cfg.min)
	return nil
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.covermode, "covermode", "count", "go test -covermode value")
	flag.StringVar(&cfg.coverpkg, "coverpkg", "", "comma-separated import paths to measure; defaults to backend-owned packages")
	flag.StringVar(&cfg.functionalQuarantine, "functional-quarantine", "", "strict functional quarantine JSON manifest; discovers and subtracts its package/test selectors")
	flag.BoolVar(&cfg.validateFunctionalQuarantine, "validate-functional-quarantine", false, "validate a functional quarantine manifest and its selectors without running coverage")
	flag.IntVar(&cfg.jobs, "jobs", 0, "maximum concurrent go test packages; defaults to runtime CPU count for non-Windows unit coverage, 1 for Windows unit coverage, and 2 for functional coverage")
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
	flag.StringVar(&cfg.coverageBuildDiagnosticsOutput, "coverage-build-diagnostics-output", "", "optional path for a coverage compile-probe cache diagnostic JSON document")
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packageBaseline, "package-baseline", "", "newline-delimited list of backend packages temporarily exempt from the per-package minimum coverage gate; defaults by suite")
	flag.Float64Var(&cfg.packageMin, "package-min", defaultPackageCoverageMin, "minimum statement coverage required for each non-baselined backend package")
	flag.Float64Var(&cfg.packageFloorEpsilon, "package-floor-epsilon", defaultPackageFloorEpsilon, "allowed manifest package-floor drift in percentage points; only applies with -package-manifest")
	flag.StringVar(&cfg.packageFloorPolicy, "package-floor-policy", coverageFloorPolicyBlocking, "package-floor enforcement policy: blocking or advisory")
	flag.BoolVar(&cfg.detailedDiagnostics, "detailed-diagnostics", false, "include uncovered source-block details in coverage diagnostics")
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
	if policy := cfg.packageFloorPolicyValue(); policy != coverageFloorPolicyBlocking && policy != coverageFloorPolicyAdvisory {
		return fmt.Errorf("configure go coverage: -package-floor-policy must be %q or %q (got %q)", coverageFloorPolicyBlocking, coverageFloorPolicyAdvisory, cfg.packageFloorPolicy)
	}
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
	if cfg.validateFunctionalQuarantine && strings.TrimSpace(cfg.functionalQuarantine) == "" {
		return errors.New("configure functional quarantine: -validate-functional-quarantine requires -functional-quarantine")
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

func executeFunctionalQuarantineValidation(cfg config) error {
	repoRoot, err := repoRootDir()
	if err != nil {
		return err
	}
	path := functionalQuarantinePath(cfg, repoRoot)
	manifest, err := readFunctionalQuarantineFile(path)
	if err != nil {
		return err
	}
	if err := validateFunctionalQuarantineMetadata(manifest); err != nil {
		return err
	}

	discoveryConfig := config{suite: functionalCoverageSuite}
	packages, listedPackages, err := resolveFunctionalTestPackagesWithMetadata(discoveryConfig, repoRoot)
	if err != nil {
		return err
	}
	inventory, err := discoverFunctionalTestInventoryFromListedPackagesWithJobs(
		packages,
		listedPackages,
		cfg.testJobs(runtime.GOOS, runtime.NumCPU()),
	)
	if err != nil {
		return err
	}
	if err := validateFunctionalQuarantine(manifest, inventory); err != nil {
		return err
	}
	if err := verifyFunctionalTestQuarantineSelectors(
		manifest,
		cfg.timeout,
		cfg.short,
		functionalQuarantineVerificationJobs(cfg.testJobs(runtime.GOOS, runtime.NumCPU())),
		repoRoot,
	); err != nil {
		return err
	}

	fmt.Fprintf(stdoutWriter, "Functional quarantine validation: manifest=%s selectors=%d status=pass\n", filepath.ToSlash(path), len(manifest.Entries))
	return nil
}

func (cfg config) packageFloorPolicyValue() string {
	policy := strings.ToLower(strings.TrimSpace(cfg.packageFloorPolicy))
	if policy == "" {
		return coverageFloorPolicyBlocking
	}
	return policy
}

func (cfg config) packageFloorPolicyIsAdvisory() bool {
	return cfg.packageFloorPolicyValue() == coverageFloorPolicyAdvisory
}

func writeAdvisoryFloorPolicyBanner() {
	fmt.Fprintln(stderrWriter, "!!! COVERAGE FLOOR POLICY: advisory !!!")
	fmt.Fprintln(stderrWriter, "Package floors and missing-manifest findings are report-only during the test-corpus rebuild.")
	fmt.Fprintln(stderrWriter, "Set -package-floor-policy=blocking to restore blocking enforcement.")
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
	return runForOSWithCPU(cfg, targetOS, runtime.NumCPU())
}

func runForOSWithCPU(cfg config, targetOS string, logicalCPUs int) (result coverageResult, runErr error) {
	profilePath, cleanup, err := prepareCoverageProfile(cfg.profile)
	if err != nil {
		return coverageResult{}, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("remove temporary coverage profile: %w", cleanupErr))
		}
	}()
	return runCoverageProfile(cfg, targetOS, logicalCPUs, profilePath)
}

func runCoverageProfile(cfg config, targetOS string, logicalCPUs int, profilePath string) (result coverageResult, runErr error) {
	repoRoot, err := repoRootDir()
	if err != nil {
		return coverageResult{}, err
	}
	selectorVerification := startFunctionalQuarantineSelectorVerification(cfg, targetOS, logicalCPUs, repoRoot)
	if selectorVerification != nil {
		selectorVerification.ratchet = startFunctionalQuarantineRatchetVerification(
			selectorVerification.manifest,
			cfg.timeout,
			cfg.short,
			repoRoot,
		)
		defer func() {
			runErr = errors.Join(runErr, selectorVerification.waitAll())
		}()
	}

	var coverPackages []string
	var testPackages []string
	var packageDiscovery coveragePackageDiscovery
	var functionalMetadata []functionalGoListPackage
	var functionalDiscoveryStarted time.Time
	var canonicalBlocks map[string]coverageBlock
	if err := cfg.measureCoveragePhase(coveragePhaseList, func() error {
		if strings.TrimSpace(cfg.functionalQuarantine) != "" {
			coverPackages, err = resolveCoverPackages(cfg)
			if err != nil {
				return err
			}
			testPackages, functionalMetadata, functionalDiscoveryStarted, err = resolveCoverageTestPackages(cfg, repoRoot, selectorVerification)
			return err
		}
		packageDiscovery, coverPackages, testPackages, err = resolveCoverageLaneWithDiscoveryForOS(cfg, targetOS)
		return err
	}); err != nil {
		return coverageResult{}, err
	}
	coverPackages = filterCoverageRequirementPackages(cfg.suite, coverPackages)

	var prepared preparedCoverageRun
	if err := cfg.measureCoveragePhase(coveragePhasePlan, func() error {
		prepared, err = prepareCoverageProfileRun(
			cfg,
			targetOS,
			logicalCPUs,
			profilePath,
			coverPackages,
			testPackages,
			packageDiscovery,
			functionalMetadata,
			functionalDiscoveryStarted,
			selectorVerification,
		)
		return err
	}); err != nil {
		return coverageResult{}, err
	}

	runErr = cfg.measureCoveragePhase(coveragePhaseTest, func() error {
		return executeCoverageInvocationPlan(
			cfg,
			prepared.plan,
			prepared.testPackages,
			profilePath,
			prepared.repoRoot,
			coverPackages,
			"run go test coverage lane",
			prepared.expectedFunctionalInventory,
		)
	})
	if runErr != nil {
		return coverageResult{}, runErr
	}
	if err := cfg.measureCoveragePhase(coveragePhaseCanonicalize, func() error {
		var err error
		canonicalBlocks, err = canonicalizeCoverageProfileWithBlocks(profilePath, prepared.repoRoot, coverPackages)
		return err
	}); err != nil {
		return coverageResult{}, err
	}

	var evaluated evaluatedCoverageRun
	if err := cfg.measureCoveragePhase(coveragePhaseEvaluate, func() error {
		var err error
		evaluated, err = evaluateCoverageRun(cfg, profilePath, prepared.repoRoot, coverPackages, canonicalBlocks)
		return err
	}); err != nil {
		return coverageResult{}, err
	}

	cfg.beginCoveragePhase(coveragePhaseManifest)
	result, err = applyCoverageManifestGate(cfg, evaluated.result, prepared.repoRoot, evaluated.baselinePackages)
	if err != nil {
		cfg.finishCoveragePhase(coveragePhaseManifest, coveragePhaseStatusError)
		return result, err
	}
	return result, nil
}

func prepareCoverageProfileRun(
	cfg config,
	targetOS string,
	logicalCPUs int,
	profilePath string,
	coverPackages []string,
	testPackages []string,
	packageDiscovery coveragePackageDiscovery,
	functionalMetadata []functionalGoListPackage,
	functionalDiscoveryStarted time.Time,
	selectorVerification *functionalQuarantineSelectorVerification,
) (preparedCoverageRun, error) {
	return prepareCoverageRunWithFunctionalMetadata(
		cfg,
		targetOS,
		logicalCPUs,
		profilePath,
		coverPackages,
		testPackages,
		packageDiscovery.allPackages,
		functionalMetadata,
		functionalDiscoveryStarted,
		selectorVerification,
		packageDiscovery.unitPackageFiles,
	)
}

func (cfg config) testJobs(targetOS string, logicalCPUs int) int {
	if cfg.jobs > 0 {
		return cfg.jobs
	}
	if targetOS == "windows" && (cfg.suite == "" || cfg.suite == "unit") {
		// Full unit coverage instrumentation exceeds the Windows host's stable
		// memory boundary when go test builds two instrumented packages at once.
		return 1
	}
	if cfg.suite == "" || cfg.suite == "unit" {
		// Unit coverage is one instrumented go test, so non-Windows hosts can
		// use their full logical CPU budget without changing functional coverage.
		if logicalCPUs > 0 {
			return logicalCPUs
		}
		// Keep the historical shared default if the runtime cannot provide a
		// usable count. runtime.NumCPU normally guarantees a positive value.
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

func resolveCoverPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.coverpkg) != "" {
		return splitList(cfg.coverpkg, ",", false), nil
	}
	listings, err := listGoPackageListings(defaultCoveragePatterns)
	if err != nil {
		return nil, err
	}
	return filterCoveragePackageListings(listings, isBackendCoveragePackage, true)
}

func resolveTestPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.packages) != "" {
		return splitList(cfg.packages, " ", true), nil
	}
	switch cfg.suite {
	case "", unitCoverageSuite:
		return listGoPackages(unitTestPatterns, isBackendCoveragePackage, false)
	case functionalCoverageSuite:
		return listGoPackages(functionalTestPatterns, isFunctionalTestPackage, false)
	default:
		return nil, fmt.Errorf("resolve go coverage lane: unsupported suite %q", cfg.suite)
	}
}

func listGoPackages(patterns []string, include func(string) bool, requireNonTestGoFiles bool) ([]string, error) {
	listings, err := listGoPackageListings(patterns)
	if err != nil {
		return nil, err
	}
	return filterCoveragePackageListings(listings, include, requireNonTestGoFiles)
}

func compactUnitTestPackageArgs(cfg config, testPackages []string, targetOS string, packageUniverse ...[]string) []string {
	// The compact patterns are safe only for the default unit package universe:
	// custom package lists and functional packages retain their existing args.
	if targetOS != "windows" || (cfg.suite != "" && cfg.suite != "unit") || strings.TrimSpace(cfg.packages) != "" {
		return nil
	}
	var allPackages []string
	if len(packageUniverse) > 0 && len(packageUniverse[0]) > 0 {
		allPackages = append([]string(nil), packageUniverse[0]...)
	} else {
		var err error
		allPackages, err = listGoPackages([]string{"./pkg/..."}, func(string) bool { return true }, false)
		if err != nil {
			return nil
		}
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

func mergeCoverageProfiles(profilePaths []string, outputPath string, repoRoot string, coverPackages []string) error {
	_, err := mergeCoverageProfilesWithBlocks(profilePaths, outputPath, repoRoot, coverPackages)
	return err
}

func canonicalizeCoverageProfileWithBlocks(profilePath string, repoRoot string, coverPackages []string) (map[string]coverageBlock, error) {
	return mergeCoverageProfilesWithBlocks([]string{profilePath}, profilePath, repoRoot, coverPackages)
}

func mergeCoverageProfilesWithBlocks(profilePaths []string, outputPath string, repoRoot string, coverPackages []string) (map[string]coverageBlock, error) {
	coverageBlocks := make(map[string]coverageBlock)
	header := ""
	for _, profilePath := range profilePaths {
		profile, err := os.Open(profilePath)
		if err != nil {
			return nil, fmt.Errorf("read go coverage profile: %w", err)
		}
		profileHeader, profileBlocks, scanErr := scanCoverageProfile(profile, repoRoot)
		closeErr := profile.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close go coverage profile: %w", closeErr)
		}
		if header == "" {
			header = profileHeader
		} else if header != profileHeader {
			return nil, fmt.Errorf("merge go coverage profiles: mode headers differ: %q and %q", header, profileHeader)
		}
		for key, block := range profileBlocks {
			merged := coverageBlocks[key]
			if merged.statementCount != 0 && merged.statementCount != block.statementCount {
				return nil, fmt.Errorf("merge go coverage profiles: source block %s has inconsistent statement counts %d and %d", key, merged.statementCount, block.statementCount)
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
		return nil, fmt.Errorf("rewrite canonical go coverage profile: %w", err)
	}
	writer := bufio.NewWriter(output)
	_, writeErr := fmt.Fprintln(writer, header)
	selectedBlocks := make(map[string]coverageBlock, len(keys))
	for _, key := range keys {
		if writeErr != nil {
			break
		}
		block := coverageBlocks[key]
		selectedBlocks[key] = block
		_, writeErr = fmt.Fprintf(writer, "%s:%s %d %d\n", block.canonicalPath, block.rangeSpec, block.statementCount, block.executionCount)
	}
	if writeErr == nil {
		writeErr = writer.Flush()
	}
	closeErr := output.Close()
	if writeErr != nil {
		return nil, fmt.Errorf("rewrite canonical go coverage profile: %w", writeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close canonical go coverage profile: %w", closeErr)
	}
	return selectedBlocks, nil
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
